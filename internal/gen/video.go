package gen

import "context"

const (
	VideoStatusQueued     = "queued"
	VideoStatusProcessing = "processing"
	VideoStatusCompleted  = "completed"
	VideoStatusFailed     = "failed"

	VideoWorkflowTextToVideo      = "text_to_video"
	VideoWorkflowImageToVideo     = "image_to_video"
	VideoWorkflowFirstLastToVideo = "first_last_to_video"
	VideoWorkflowReferenceToVideo = "reference_to_video"
	VideoWorkflowMotionTransfer   = "motion_transfer"
	VideoWorkflowOmniMultimodal   = "omni_multimodal"

	VideoSlotStartImage      = "start_image"
	VideoSlotEndImage        = "end_image"
	VideoSlotReferenceImages = "reference_images"
	VideoSlotReferenceVideos = "reference_videos"
	VideoSlotReferenceAudios = "reference_audios"
	VideoSlotDriverVideo     = "driver_video"
	VideoSlotDriverAudio     = "driver_audio"

	VideoReferenceModeAuto              = "auto"
	VideoReferenceModeNone              = "none"
	VideoReferenceModeIdentityReference = "identity_reference"
	VideoReferenceModeFirstFrame        = "first_frame"
	VideoReferenceModeStartEndFrame     = "start_end_frame"
)

// VideoMediaInput is one typed media reference sent to a provider adapter.
type VideoMediaInput struct {
	Data  []byte
	URL   string
	MIME  string
	Label string
}

// VideoRequest describes a provider-neutral image-to-video generation task.
type VideoRequest struct {
	Model           string
	Prompt          string
	NegativePrompt  string
	Workflow        string
	ReferenceImage  []byte
	ReferenceMode   string
	MediaSlots      map[string][]VideoMediaInput
	ReferenceVideos []string
	ReferenceAudios []string
	Controls        map[string]any
	Seconds         int
	FPS             int
	Size            string
	AspectRatio     string
	Resolution      string
	ProviderAdapter string
}

// VideoResult is a provider-neutral video task snapshot.
type VideoResult struct {
	ID       string
	Status   string
	URL      string
	Progress int
	Message  string
}

// Done reports whether the provider task reached a terminal successful state.
func (r VideoResult) Done() bool {
	return r.Status == VideoStatusCompleted
}

// Failed reports whether the provider task reached a terminal failure state.
func (r VideoResult) Failed() bool {
	return r.Status == VideoStatusFailed
}

// VideoProvider is implemented by providers that can create and retrieve video tasks.
type VideoProvider interface {
	GenerateVideo(ctx context.Context, req VideoRequest) (VideoResult, error)
	PollVideo(ctx context.Context, taskID string) (VideoResult, error)
	DownloadVideo(ctx context.Context, idOrURL string) ([]byte, error)
}
