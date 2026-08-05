package main

import "encoding/json"

// ToolchainSchemaVersion is incremented only when persisted toolchain metadata
// needs an incompatible migration.
const ToolchainSchemaVersion = 1

// AssetMediaType is the broad media category used by the shared asset library.
type AssetMediaType string

const (
	AssetMediaImage   AssetMediaType = "image"
	AssetMediaVideo   AssetMediaType = "video"
	AssetMediaAudio   AssetMediaType = "audio"
	AssetMediaText    AssetMediaType = "text"
	AssetMediaJSON    AssetMediaType = "json"
	AssetMediaArchive AssetMediaType = "archive"
)

// ToolchainModule identifies the workflow that created an asset or job.
type ToolchainModule string

const (
	ToolchainModuleAnimation ToolchainModule = "animation"
	ToolchainModuleImage     ToolchainModule = "image"
	ToolchainModuleVideo     ToolchainModule = "video"
	ToolchainModuleUI        ToolchainModule = "ui"
)

// ToolAsset is the public metadata returned to the frontend. Filesystem paths
// are deliberately kept in the private persisted envelope and resolved by ID.
type ToolAsset struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	MediaType          AssetMediaType  `json:"mediaType"`
	MIMEType           string          `json:"mimeType,omitempty"`
	Usage              string          `json:"usage,omitempty"`
	SizeBytes          int64           `json:"sizeBytes,omitempty"`
	Width              int             `json:"width,omitempty"`
	Height             int             `json:"height,omitempty"`
	DurationMs         int64           `json:"durationMs,omitempty"`
	ThumbnailAvailable bool            `json:"thumbnailAvailable,omitempty"`
	SourceModule       ToolchainModule `json:"sourceModule,omitempty"`
	ParentAssetID      string          `json:"parentAssetId,omitempty"`
	Prompt             string          `json:"prompt,omitempty"`
	Provider           string          `json:"provider,omitempty"`
	Model              string          `json:"model,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	LegacyIndexed      bool            `json:"legacyIndexed,omitempty"`
	CreatedAt          string          `json:"createdAt"`
	UpdatedAt          string          `json:"updatedAt"`
}

// GenerationJobKind identifies the operation represented by a persistent job.
type GenerationJobKind string

const (
	GenerationJobImage              GenerationJobKind = "image"
	GenerationJobVideo              GenerationJobKind = "video"
	GenerationJobUIAnalysis         GenerationJobKind = "ui_analysis"
	GenerationJobUISplit            GenerationJobKind = "ui_split"
	GenerationJobPromptOptimization GenerationJobKind = "prompt_optimization"

	// Short aliases keep provider and App adapters readable.
	JobKindImage              = GenerationJobImage
	JobKindVideo              = GenerationJobVideo
	JobKindUIAnalysis         = GenerationJobUIAnalysis
	JobKindUISplit            = GenerationJobUISplit
	JobKindPromptOptimization = GenerationJobPromptOptimization
)

// GenerationJobStatus is intentionally explicit because restart and
// cancellation behavior differs before and after a paid provider submission.
type GenerationJobStatus string

const (
	GenerationJobQueued      GenerationJobStatus = "queued"
	GenerationJobRunning     GenerationJobStatus = "running"
	GenerationJobSubmitting  GenerationJobStatus = "submitting"
	GenerationJobPolling     GenerationJobStatus = "polling"
	GenerationJobCompleted   GenerationJobStatus = "completed"
	GenerationJobFailed      GenerationJobStatus = "failed"
	GenerationJobCancelled   GenerationJobStatus = "cancelled"
	GenerationJobStopped     GenerationJobStatus = "stopped"
	GenerationJobNeedsReview GenerationJobStatus = "needs_review"

	JobStatusQueued      = GenerationJobQueued
	JobStatusRunning     = GenerationJobRunning
	JobStatusSubmitting  = GenerationJobSubmitting
	JobStatusPolling     = GenerationJobPolling
	JobStatusCompleted   = GenerationJobCompleted
	JobStatusFailed      = GenerationJobFailed
	JobStatusCancelled   = GenerationJobCancelled
	JobStatusStopped     = GenerationJobStopped
	JobStatusNeedsReview = GenerationJobNeedsReview
)

// Terminal reports whether a job can no longer make progress without creating
// a new retry job. Needs-review is intentionally not terminal: the UI can still
// reconcile it with an upstream task before choosing to cancel or retry.
func (s GenerationJobStatus) Terminal() bool {
	switch s {
	case GenerationJobCompleted, GenerationJobFailed, GenerationJobCancelled, GenerationJobStopped:
		return true
	default:
		return false
	}
}

// GenerationJob is the durable task-center record. Request is a sanitized JSON
// snapshot; providers must never place API keys or authorization headers in it.
type GenerationJob struct {
	ID                      string              `json:"id"`
	Kind                    GenerationJobKind   `json:"kind"`
	Status                  GenerationJobStatus `json:"status"`
	Phase                   string              `json:"phase,omitempty"`
	Progress                float64             `json:"progress"`
	Provider                string              `json:"provider,omitempty"`
	Model                   string              `json:"model,omitempty"`
	ProviderTaskID          string              `json:"providerTaskId,omitempty"`
	InputAssetIDs           []string            `json:"inputAssetIds,omitempty"`
	OutputAssetIDs          []string            `json:"outputAssetIds,omitempty"`
	Error                   string              `json:"error,omitempty"`
	Request                 map[string]any      `json:"request,omitempty"`
	SourceModule            ToolchainModule     `json:"sourceModule,omitempty"`
	RetryOfJobID            string              `json:"retryOfJobId,omitempty"`
	Attempt                 int                 `json:"attempt"`
	UpstreamMayContinue     bool                `json:"upstreamMayContinue,omitempty"`
	CreatedAt               string              `json:"createdAt"`
	UpdatedAt               string              `json:"updatedAt"`
	StartedAt               string              `json:"startedAt,omitempty"`
	CompletedAt             string              `json:"completedAt,omitempty"`
	ProviderResponseSummary json.RawMessage     `json:"providerResponseSummary,omitempty"`
}

// UIRegion uses source-image pixel coordinates so it remains stable at any UI
// zoom level. Style is deliberately open-ended for provider-derived tokens.
type UIRegion struct {
	ID            string         `json:"id"`
	ParentID      string         `json:"parentId,omitempty"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Bounds        UIPixelBounds  `json:"bounds"`
	ZIndex        int            `json:"zIndex"`
	Confidence    float64        `json:"confidence"`
	Text          string         `json:"text,omitempty"`
	Style         map[string]any `json:"style,omitempty"`
	ExportEnabled bool           `json:"exportEnabled"`
}

type UIPixelBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}
