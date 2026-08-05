package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	defaultConcurrentToolchainJobs = 6
	maxJobRequestSnapshotBytes     = 1 << 20
	jobCancelTimeout               = 10 * time.Second
)

var (
	ErrJobManagerNotStarted     = errors.New("generation job manager is not started")
	ErrJobManagerClosed         = errors.New("generation job manager is closed")
	ErrJobAlreadyRunning        = errors.New("generation job is already running")
	ErrInvalidJobTransition     = errors.New("invalid generation job status transition")
	ErrSubmissionOutcomeUnknown = errors.New("provider submission outcome is unknown")
)

type GenerationJobRunFunc func(context.Context, GenerationJob, *GenerationJobControl) error
type GenerationJobCloneFunc func(GenerationJob) (GenerationJob, error)
type GenerationJobCancelFunc func(context.Context, GenerationJob) error

// GenerationJobHandler separates a fresh paid submission from restart polling.
// Resume must only inspect an already-known ProviderTaskID; it must never create
// a new provider task.
type GenerationJobHandler struct {
	Run    GenerationJobRunFunc
	Resume GenerationJobRunFunc
	Clone  GenerationJobCloneFunc
	Cancel GenerationJobCancelFunc
}

type GenerationJobEventType string

const (
	GenerationJobEventCreated GenerationJobEventType = "created"
	GenerationJobEventUpdated GenerationJobEventType = "updated"
	GenerationJobEventDeleted GenerationJobEventType = "deleted"
)

type GenerationJobEvent struct {
	Type GenerationJobEventType `json:"type"`
	Job  GenerationJob          `json:"job"`
}

type GenerationJobEventCallback func(GenerationJobEvent)

type GenerationJobManagerOptions struct {
	MaxConcurrentJobs int
	OnEvent           GenerationJobEventCallback
}

type GenerationJobManager struct {
	store *ToolchainStore

	mu            sync.Mutex
	jobs          map[string]GenerationJob
	handlers      map[GenerationJobKind]GenerationJobHandler
	cancels       map[string]context.CancelFunc
	providerGates map[string]chan struct{}
	semaphore     chan struct{}
	onEvent       GenerationJobEventCallback
	ctx           context.Context
	cancel        context.CancelFunc
	started       bool
	closed        bool
	wg            sync.WaitGroup
}

// GenerationJobUpdate uses zero values as "unchanged", except Progress which
// is updated when SetProgress is true and OutputAssetIDs which is replaced when
// non-nil.
type GenerationJobUpdate struct {
	Status                  GenerationJobStatus
	Phase                   string
	SetPhase                bool
	Progress                float64
	SetProgress             bool
	ProviderTaskID          string
	SetProviderTaskID       bool
	OutputAssetIDs          []string
	Error                   string
	SetError                bool
	UpstreamMayContinue     bool
	SetUpstreamMayContinue  bool
	ProviderResponseSummary json.RawMessage
	SetResponseSummary      bool
}

// GenerationJobControl is scoped to one running job. Submit is the only paid
// submission primitive: it serializes creation requests per provider while
// allowing polling and local post-processing jobs to continue concurrently.
type GenerationJobControl struct {
	manager *GenerationJobManager
	jobID   string
	ctx     context.Context
}

type ProviderSubmissionFunc func(context.Context) (providerTaskID string, err error)

func NewGenerationJobManager(store *ToolchainStore, options GenerationJobManagerOptions) (*GenerationJobManager, error) {
	if store == nil {
		return nil, errors.New("toolchain store is nil")
	}
	jobs, err := store.ListGenerationJobs()
	if err != nil {
		return nil, err
	}
	maxConcurrent := options.MaxConcurrentJobs
	if maxConcurrent <= 0 {
		maxConcurrent = defaultConcurrentToolchainJobs
	}
	m := &GenerationJobManager{
		store:         store,
		jobs:          make(map[string]GenerationJob, len(jobs)),
		handlers:      make(map[GenerationJobKind]GenerationJobHandler),
		cancels:       make(map[string]context.CancelFunc),
		providerGates: make(map[string]chan struct{}),
		semaphore:     make(chan struct{}, maxConcurrent),
		onEvent:       options.OnEvent,
	}
	for _, job := range jobs {
		m.jobs[job.ID] = cloneGenerationJob(job)
	}
	return m, nil
}

// RegisterHandler should be called before Start so restart recovery knows which
// active provider tasks are safe to resume.
func (m *GenerationJobManager) RegisterHandler(kind GenerationJobKind, handler GenerationJobHandler) error {
	if strings.TrimSpace(string(kind)) == "" {
		return errors.New("job kind is empty")
	}
	if handler.Run == nil && handler.Resume == nil {
		return errors.New("job handler has neither run nor resume callback")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrJobManagerClosed
	}
	m.handlers[kind] = handler
	return nil
}

// Start applies conservative restart rules and begins only resumable remote
// polling. Persisted queued jobs stay queued until StartQueuedJob is explicit.
func (m *GenerationJobManager) Start(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrJobManagerClosed
	}
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(parent)
	m.started = true
	jobs := make([]GenerationJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, cloneGenerationJob(job))
	}
	m.mu.Unlock()

	for _, job := range jobs {
		switch job.Status {
		case GenerationJobQueued:
			// Queued may represent a paid POST that never started. Never replay it.
			continue
		case GenerationJobSubmitting, GenerationJobRunning, GenerationJobPolling:
			m.mu.Lock()
			handler, registered := m.handlers[job.Kind]
			m.mu.Unlock()
			if job.ProviderTaskID != "" && registered && handler.Resume != nil {
				_, _ = m.UpdateJob(job.ID, GenerationJobUpdate{
					Status:   GenerationJobRunning,
					Phase:    "resuming",
					SetPhase: true,
					Error:    "",
					SetError: true,
				})
				_ = m.launchJob(job.ID, handler.Resume, true)
				continue
			}
			reason := "job was interrupted before a resumable provider task id was persisted"
			if job.ProviderTaskID != "" {
				reason = "provider task exists but no safe resume callback is registered"
			}
			_, _ = m.UpdateJob(job.ID, GenerationJobUpdate{
				Status:                 GenerationJobNeedsReview,
				Phase:                  "needs_review",
				SetPhase:               true,
				Error:                  reason,
				SetError:               true,
				UpstreamMayContinue:    true,
				SetUpstreamMayContinue: true,
			})
		}
	}
	return nil
}

func (m *GenerationJobManager) CreateJob(job GenerationJob) (GenerationJob, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return GenerationJob{}, ErrJobManagerClosed
	}
	if !m.started {
		m.mu.Unlock()
		return GenerationJob{}, ErrJobManagerNotStarted
	}
	handler, registered := m.handlers[job.Kind]
	m.mu.Unlock()
	if !registered || handler.Run == nil {
		return GenerationJob{}, fmt.Errorf("no run handler registered for job kind %q", job.Kind)
	}

	request, err := normalizeJobRequest(job.Request)
	if err != nil {
		return GenerationJob{}, err
	}
	job.Request = request
	if job.ID == "" {
		job.ID = newToolchainID("job")
	}
	if !validToolchainID(job.ID) {
		return GenerationJob{}, ErrInvalidToolchainID
	}
	if strings.TrimSpace(string(job.Kind)) == "" {
		return GenerationJob{}, errors.New("job kind is empty")
	}
	for _, id := range job.InputAssetIDs {
		if !validToolchainID(id) {
			return GenerationJob{}, fmt.Errorf("invalid input asset id %q", id)
		}
	}
	job.Status = GenerationJobQueued
	job.Phase = "queued"
	job.Progress = 0
	job.ProviderTaskID = ""
	job.OutputAssetIDs = nil
	job.Error = ""
	job.UpstreamMayContinue = false
	job.StartedAt = ""
	job.CompletedAt = ""
	job.ProviderResponseSummary = nil
	if job.Attempt <= 0 {
		job.Attempt = 1
	}
	now := toolchainTimestamp(time.Now())
	job.CreatedAt = now
	job.UpdatedAt = now
	job.InputAssetIDs = normalizedStrings(job.InputAssetIDs)

	m.mu.Lock()
	if _, exists := m.jobs[job.ID]; exists {
		m.mu.Unlock()
		return GenerationJob{}, fmt.Errorf("job %q already exists", job.ID)
	}
	if err := m.store.SaveGenerationJob(job); err != nil {
		m.mu.Unlock()
		return GenerationJob{}, err
	}
	m.jobs[job.ID] = cloneGenerationJob(job)
	m.mu.Unlock()
	m.emit(GenerationJobEvent{Type: GenerationJobEventCreated, Job: job})

	if err := m.launchJob(job.ID, handler.Run, false); err != nil {
		_, _ = m.UpdateJob(job.ID, GenerationJobUpdate{Status: GenerationJobFailed, Error: err.Error(), SetError: true})
		return GenerationJob{}, err
	}
	return cloneGenerationJob(job), nil
}

// StartQueuedJob is the only way to run a queued job loaded from disk. This
// explicit action prevents automatic replay of a potentially paid request.
func (m *GenerationJobManager) StartQueuedJob(id string) (GenerationJob, error) {
	job, err := m.GetJob(id)
	if err != nil {
		return GenerationJob{}, err
	}
	if job.Status != GenerationJobQueued {
		return GenerationJob{}, fmt.Errorf("job %q is not queued", id)
	}
	m.mu.Lock()
	handler, exists := m.handlers[job.Kind]
	m.mu.Unlock()
	if !exists || handler.Run == nil {
		return GenerationJob{}, fmt.Errorf("no run handler registered for job kind %q", job.Kind)
	}
	if err := m.launchJob(id, handler.Run, false); err != nil {
		return GenerationJob{}, err
	}
	return m.GetJob(id)
}

func (m *GenerationJobManager) GetJob(id string) (GenerationJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, exists := m.jobs[id]
	if !exists {
		return GenerationJob{}, ErrToolchainNotFound
	}
	return cloneGenerationJob(job), nil
}

func (m *GenerationJobManager) ListJobs() []GenerationJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]GenerationJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, cloneGenerationJob(job))
	}
	sortGenerationJobs(jobs)
	return jobs
}

func sortGenerationJobs(jobs []GenerationJob) {
	for i := 1; i < len(jobs); i++ {
		for j := i; j > 0 && jobs[j].CreatedAt > jobs[j-1].CreatedAt; j-- {
			jobs[j], jobs[j-1] = jobs[j-1], jobs[j]
		}
	}
}

func (m *GenerationJobManager) UpdateJob(id string, update GenerationJobUpdate) (GenerationJob, error) {
	m.mu.Lock()
	job, exists := m.jobs[id]
	if !exists {
		m.mu.Unlock()
		return GenerationJob{}, ErrToolchainNotFound
	}
	next, err := applyGenerationJobUpdate(job, update)
	if err != nil {
		m.mu.Unlock()
		return GenerationJob{}, err
	}
	if err := m.store.SaveGenerationJob(next); err != nil {
		m.mu.Unlock()
		return GenerationJob{}, err
	}
	m.jobs[id] = cloneGenerationJob(next)
	m.mu.Unlock()
	m.emit(GenerationJobEvent{Type: GenerationJobEventUpdated, Job: next})
	return cloneGenerationJob(next), nil
}

func (m *GenerationJobManager) CancelJob(id string) (GenerationJob, error) {
	job, err := m.GetJob(id)
	if err != nil {
		return GenerationJob{}, err
	}
	if job.Status.Terminal() {
		return job, nil
	}
	m.mu.Lock()
	cancel := m.cancels[id]
	handler := m.handlers[job.Kind]
	m.mu.Unlock()

	var update GenerationJobUpdate
	switch {
	case job.Status == GenerationJobQueued:
		update = terminalJobUpdate(GenerationJobCancelled, "cancelled", "", false)
	case job.Status == GenerationJobSubmitting && job.ProviderTaskID == "":
		update = GenerationJobUpdate{
			Status:                 GenerationJobNeedsReview,
			Phase:                  "needs_review",
			SetPhase:               true,
			Error:                  "submission was interrupted before its provider task id was known",
			SetError:               true,
			UpstreamMayContinue:    true,
			SetUpstreamMayContinue: true,
		}
	case job.ProviderTaskID != "":
		if handler.Cancel == nil {
			update = terminalJobUpdate(GenerationJobStopped, "stopped", "local waiting stopped; the upstream provider task may continue", true)
			break
		}
		ctx, cancelRemote := context.WithTimeout(context.Background(), jobCancelTimeout)
		cancelErr := handler.Cancel(ctx, job)
		cancelRemote()
		latest, getErr := m.GetJob(id)
		if getErr == nil && latest.Status.Terminal() {
			if cancel != nil {
				cancel()
			}
			return latest, nil
		}
		if cancelErr != nil {
			update = terminalJobUpdate(GenerationJobStopped, "stopped", "provider cancellation failed; upstream task may continue: "+cancelErr.Error(), true)
		} else {
			update = terminalJobUpdate(GenerationJobCancelled, "cancelled", "", false)
		}
	default:
		update = terminalJobUpdate(GenerationJobCancelled, "cancelled", "", false)
	}
	updated, err := m.UpdateJob(id, update)
	if cancel != nil {
		cancel()
	}
	return updated, err
}

func (m *GenerationJobManager) RetryJob(id string) (GenerationJob, error) {
	original, err := m.GetJob(id)
	if err != nil {
		return GenerationJob{}, err
	}
	if !original.Status.Terminal() && original.Status != GenerationJobNeedsReview {
		return GenerationJob{}, errors.New("only finished or needs-review jobs can be retried")
	}
	m.mu.Lock()
	handler, exists := m.handlers[original.Kind]
	m.mu.Unlock()
	if !exists || handler.Clone == nil {
		return GenerationJob{}, fmt.Errorf("no clone callback registered for job kind %q", original.Kind)
	}
	clone, err := handler.Clone(cloneGenerationJob(original))
	if err != nil {
		return GenerationJob{}, err
	}
	clone.ID = ""
	if clone.Kind == "" {
		clone.Kind = original.Kind
	}
	clone.RetryOfJobID = original.ID
	clone.Attempt = original.Attempt + 1
	return m.CreateJob(clone)
}

func (m *GenerationJobManager) DeleteJob(id string) error {
	job, err := m.GetJob(id)
	if err != nil {
		return err
	}
	if !job.Status.Terminal() && job.Status != GenerationJobNeedsReview && job.Status != GenerationJobQueued {
		return errors.New("active jobs cannot be deleted")
	}
	m.mu.Lock()
	if _, running := m.cancels[id]; running {
		m.mu.Unlock()
		return ErrJobAlreadyRunning
	}
	if err := m.store.DeleteGenerationJob(id); err != nil {
		m.mu.Unlock()
		return err
	}
	delete(m.jobs, id)
	m.mu.Unlock()
	m.emit(GenerationJobEvent{Type: GenerationJobEventDeleted, Job: job})
	return nil
}

// Close stops local goroutines but intentionally preserves active persisted
// statuses so a future process can resume known provider task IDs safely.
func (m *GenerationJobManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *GenerationJobManager) launchJob(id string, run GenerationJobRunFunc, resume bool) error {
	if run == nil {
		return errors.New("job runner is nil")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrJobManagerClosed
	}
	if !m.started || m.ctx == nil {
		m.mu.Unlock()
		return ErrJobManagerNotStarted
	}
	if _, exists := m.cancels[id]; exists {
		m.mu.Unlock()
		return ErrJobAlreadyRunning
	}
	if _, exists := m.jobs[id]; !exists {
		m.mu.Unlock()
		return ErrToolchainNotFound
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancels[id] = cancel
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			m.mu.Lock()
			delete(m.cancels, id)
			m.mu.Unlock()
			m.wg.Done()
		}()

		select {
		case m.semaphore <- struct{}{}:
			defer func() { <-m.semaphore }()
		case <-ctx.Done():
			m.finalizeJob(ctx, id, ctx.Err())
			return
		}

		if !resume {
			if _, err := m.UpdateJob(id, GenerationJobUpdate{
				Status:      GenerationJobRunning,
				Phase:       "starting",
				SetPhase:    true,
				Progress:    0,
				SetProgress: true,
			}); err != nil {
				m.finalizeJob(ctx, id, err)
				return
			}
		}
		job, err := m.GetJob(id)
		if err != nil || job.Status.Terminal() || job.Status == GenerationJobNeedsReview {
			return
		}
		control := &GenerationJobControl{manager: m, jobID: id, ctx: ctx}
		err = run(ctx, job, control)
		m.finalizeJob(ctx, id, err)
	}()
	return nil
}

func (m *GenerationJobManager) finalizeJob(ctx context.Context, id string, runErr error) {
	job, err := m.GetJob(id)
	if err != nil || job.Status.Terminal() || job.Status == GenerationJobNeedsReview {
		return
	}
	if runErr == nil {
		_, _ = m.UpdateJob(id, terminalJobUpdate(GenerationJobCompleted, "completed", "", false))
		return
	}
	if errors.Is(runErr, ErrSubmissionOutcomeUnknown) {
		_, _ = m.UpdateJob(id, GenerationJobUpdate{
			Status:                 GenerationJobNeedsReview,
			Phase:                  "needs_review",
			SetPhase:               true,
			Error:                  boundedJobError(runErr),
			SetError:               true,
			UpstreamMayContinue:    true,
			SetUpstreamMayContinue: true,
		})
		return
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || ctx.Err() != nil {
		m.mu.Lock()
		managerStopped := m.ctx == nil || m.ctx.Err() != nil
		m.mu.Unlock()
		if managerStopped {
			return
		}
		if job.Status == GenerationJobSubmitting && job.ProviderTaskID == "" {
			_, _ = m.UpdateJob(id, GenerationJobUpdate{
				Status:                 GenerationJobNeedsReview,
				Phase:                  "needs_review",
				SetPhase:               true,
				Error:                  "submission stopped before its provider task id was known",
				SetError:               true,
				UpstreamMayContinue:    true,
				SetUpstreamMayContinue: true,
			})
		} else if job.ProviderTaskID != "" {
			_, _ = m.UpdateJob(id, terminalJobUpdate(GenerationJobStopped, "stopped", "local waiting stopped; the upstream provider task may continue", true))
		} else {
			_, _ = m.UpdateJob(id, terminalJobUpdate(GenerationJobCancelled, "cancelled", "", false))
		}
		return
	}
	_, _ = m.UpdateJob(id, terminalJobUpdate(GenerationJobFailed, "failed", boundedJobError(runErr), false))
}

func (c *GenerationJobControl) Current() (GenerationJob, error) {
	return c.manager.GetJob(c.jobID)
}

func (c *GenerationJobControl) Update(update GenerationJobUpdate) (GenerationJob, error) {
	return c.manager.UpdateJob(c.jobID, update)
}

func (c *GenerationJobControl) SetProgress(phase string, progress float64) (GenerationJob, error) {
	return c.Update(GenerationJobUpdate{Phase: phase, SetPhase: true, Progress: progress, SetProgress: true})
}

func (c *GenerationJobControl) SetPolling(phase string, progress float64) (GenerationJob, error) {
	return c.Update(GenerationJobUpdate{
		Status:      GenerationJobPolling,
		Phase:       phase,
		SetPhase:    true,
		Progress:    progress,
		SetProgress: true,
	})
}

func (c *GenerationJobControl) SetOutputAssetIDs(ids []string) (GenerationJob, error) {
	for _, id := range ids {
		if !validToolchainID(id) {
			return GenerationJob{}, fmt.Errorf("invalid output asset id %q", id)
		}
	}
	return c.Update(GenerationJobUpdate{OutputAssetIDs: normalizedStrings(ids)})
}

func (c *GenerationJobControl) SetProviderResponseSummary(summary json.RawMessage) (GenerationJob, error) {
	if len(summary) > maxJobRequestSnapshotBytes {
		return GenerationJob{}, errors.New("provider response summary is too large")
	}
	if len(summary) > 0 && !json.Valid(summary) {
		return GenerationJob{}, errors.New("provider response summary is not valid JSON")
	}
	return c.Update(GenerationJobUpdate{ProviderResponseSummary: summary, SetResponseSummary: true})
}

func (c *GenerationJobControl) Submit(submit ProviderSubmissionFunc) (string, error) {
	if submit == nil {
		return "", errors.New("provider submission callback is nil")
	}
	job, err := c.Current()
	if err != nil {
		return "", err
	}
	_, _ = c.Update(GenerationJobUpdate{Phase: "waiting_provider", SetPhase: true})
	gate := c.manager.providerGate(job.Provider)
	if gate != nil {
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		case <-c.ctx.Done():
			return "", c.ctx.Err()
		}
	}
	if err := c.ctx.Err(); err != nil {
		return "", err
	}
	if _, err := c.Update(GenerationJobUpdate{
		Status:   GenerationJobSubmitting,
		Phase:    "submitting",
		SetPhase: true,
		Error:    "",
		SetError: true,
	}); err != nil {
		return "", err
	}
	taskID, err := submit(c.ctx)
	if err != nil {
		if errors.Is(err, ErrSubmissionOutcomeUnknown) {
			_, _ = c.Update(GenerationJobUpdate{
				Status:                 GenerationJobNeedsReview,
				Phase:                  "needs_review",
				SetPhase:               true,
				Error:                  boundedJobError(err),
				SetError:               true,
				UpstreamMayContinue:    true,
				SetUpstreamMayContinue: true,
			})
		}
		return "", err
	}
	taskID = strings.TrimSpace(taskID)
	if _, err := c.Update(GenerationJobUpdate{
		Status:                 GenerationJobRunning,
		Phase:                  "submitted",
		SetPhase:               true,
		ProviderTaskID:         taskID,
		SetProviderTaskID:      true,
		UpstreamMayContinue:    false,
		SetUpstreamMayContinue: true,
	}); err != nil {
		return taskID, fmt.Errorf("%w: provider task %q was created but could not be persisted: %v", ErrSubmissionOutcomeUnknown, taskID, err)
	}
	return taskID, nil
}

func (m *GenerationJobManager) providerGate(provider string) chan struct{} {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	gate := m.providerGates[provider]
	if gate == nil {
		gate = make(chan struct{}, 1)
		m.providerGates[provider] = gate
	}
	return gate
}

func (m *GenerationJobManager) emit(event GenerationJobEvent) {
	if m.onEvent == nil {
		return
	}
	event.Job = cloneGenerationJob(event.Job)
	defer func() { _ = recover() }()
	m.onEvent(event)
}

func applyGenerationJobUpdate(job GenerationJob, update GenerationJobUpdate) (GenerationJob, error) {
	next := cloneGenerationJob(job)
	if update.Status != "" && update.Status != job.Status {
		if !canTransitionGenerationJob(job.Status, update.Status) {
			return GenerationJob{}, fmt.Errorf("%w: %s -> %s", ErrInvalidJobTransition, job.Status, update.Status)
		}
		next.Status = update.Status
	}
	if update.SetPhase {
		next.Phase = strings.TrimSpace(update.Phase)
	}
	if update.SetProgress {
		if math.IsNaN(update.Progress) || math.IsInf(update.Progress, 0) {
			return GenerationJob{}, errors.New("job progress must be finite")
		}
		next.Progress = max(0, min(1, update.Progress))
	}
	if update.SetProviderTaskID {
		next.ProviderTaskID = strings.TrimSpace(update.ProviderTaskID)
	}
	if update.OutputAssetIDs != nil {
		for _, id := range update.OutputAssetIDs {
			if !validToolchainID(id) {
				return GenerationJob{}, fmt.Errorf("invalid output asset id %q", id)
			}
		}
		next.OutputAssetIDs = normalizedStrings(update.OutputAssetIDs)
	}
	if update.SetError {
		next.Error = strings.TrimSpace(update.Error)
	}
	if update.SetUpstreamMayContinue {
		next.UpstreamMayContinue = update.UpstreamMayContinue
	}
	if update.SetResponseSummary {
		next.ProviderResponseSummary = append(json.RawMessage(nil), update.ProviderResponseSummary...)
	}
	now := toolchainTimestamp(time.Now())
	next.UpdatedAt = now
	if next.Status != GenerationJobQueued && next.StartedAt == "" {
		next.StartedAt = now
	}
	if next.Status.Terminal() {
		next.CompletedAt = now
		if next.Status == GenerationJobCompleted {
			next.Progress = 1
		}
	}
	return next, nil
}

func canTransitionGenerationJob(from, to GenerationJobStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case GenerationJobQueued:
		return to == GenerationJobRunning || to == GenerationJobCancelled || to == GenerationJobFailed
	case GenerationJobRunning:
		return to == GenerationJobSubmitting || to == GenerationJobPolling || to == GenerationJobCompleted || to == GenerationJobFailed || to == GenerationJobCancelled || to == GenerationJobStopped || to == GenerationJobNeedsReview
	case GenerationJobSubmitting:
		return to == GenerationJobRunning || to == GenerationJobPolling || to == GenerationJobCompleted || to == GenerationJobFailed || to == GenerationJobCancelled || to == GenerationJobStopped || to == GenerationJobNeedsReview
	case GenerationJobPolling:
		return to == GenerationJobRunning || to == GenerationJobCompleted || to == GenerationJobFailed || to == GenerationJobCancelled || to == GenerationJobStopped || to == GenerationJobNeedsReview
	case GenerationJobNeedsReview:
		return to == GenerationJobCancelled || to == GenerationJobStopped || to == GenerationJobFailed || to == GenerationJobCompleted || to == GenerationJobRunning
	default:
		return false
	}
}

func terminalJobUpdate(status GenerationJobStatus, phase, message string, upstreamMayContinue bool) GenerationJobUpdate {
	return GenerationJobUpdate{
		Status:                 status,
		Phase:                  phase,
		SetPhase:               true,
		Error:                  message,
		SetError:               true,
		UpstreamMayContinue:    upstreamMayContinue,
		SetUpstreamMayContinue: true,
	}
}

func normalizeJobRequest(request map[string]any) (map[string]any, error) {
	if request == nil {
		return nil, nil
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode job request snapshot: %w", err)
	}
	if len(data) > maxJobRequestSnapshotBytes {
		return nil, fmt.Errorf("job request snapshot exceeds %d bytes", maxJobRequestSnapshotBytes)
	}
	var normalized map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("normalize job request snapshot: %w", err)
	}
	redactRequestSecrets(normalized)
	return normalized, nil
}

func redactRequestSecrets(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if secretRequestKey(key) {
				typed[key] = "[redacted]"
				continue
			}
			redactRequestSecrets(child)
		}
	case []any:
		for _, child := range typed {
			redactRequestSecrets(child)
		}
	}
}

func secretRequestKey(key string) bool {
	key = strings.ToLower(key)
	key = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(key)
	for _, fragment := range []string{"authorization", "apikey", "accesstoken", "refreshtoken", "password", "secret", "credential", "cookie"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func boundedJobError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	const maxErrorBytes = 4096
	if len(message) > maxErrorBytes {
		message = message[:maxErrorBytes]
	}
	return message
}
