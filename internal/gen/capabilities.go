package gen

import "strings"

type ImageModelCapability struct {
	Model                   string   `json:"model"`
	SupportsTextToImage     bool     `json:"supportsTextToImage"`
	SupportsReferenceImages bool     `json:"supportsReferenceImages"`
	SupportsMaskEdit        bool     `json:"supportsMaskEdit"`
	MaxReferenceImages      int      `json:"maxReferenceImages"`
	AllowedAspectRatios     []string `json:"allowedAspectRatios"`
}

type GenerationCapabilities struct {
	ImageProviders  map[string]map[string]ImageModelCapability `json:"imageProviders"`
	VideoProviders  map[string]map[string]VideoModelCapability `json:"videoProviders"`
	VisionProviders map[string][]string                        `json:"visionProviders"`
}

func ImageModelCapabilityFor(provider, model string) ImageModelCapability {
	m := strings.ToLower(strings.TrimSpace(model))
	cap := ImageModelCapability{
		Model:                   strings.TrimSpace(model),
		SupportsTextToImage:     true,
		SupportsReferenceImages: true,
		SupportsMaskEdit:        false,
		MaxReferenceImages:      4,
		AllowedAspectRatios:     []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"},
	}
	if provider == ProviderFal && strings.Contains(m, "flux") {
		cap.MaxReferenceImages = 1
	}
	if provider == ProviderBytePlus && strings.Contains(m, "t2i") {
		cap.SupportsReferenceImages = false
		cap.MaxReferenceImages = 0
	}
	if provider == ProviderOpenAI || provider == ProviderZetaTechs {
		cap.SupportsMaskEdit = strings.HasPrefix(m, "gpt-image")
	}
	return cap
}

func CapabilitiesForCatalog() GenerationCapabilities {
	out := GenerationCapabilities{
		ImageProviders:  map[string]map[string]ImageModelCapability{},
		VideoProviders:  map[string]map[string]VideoModelCapability{},
		VisionProviders: map[string][]string{},
	}
	for _, provider := range SupportedProviders {
		images := map[string]ImageModelCapability{}
		for _, model := range ModelsFor(provider) {
			images[model] = ImageModelCapabilityFor(provider, model)
		}
		if len(images) > 0 {
			out.ImageProviders[provider] = images
		}
		videos := map[string]VideoModelCapability{}
		for _, model := range VideoModelsFor(provider) {
			videos[model] = VideoModelCapabilityFor(model)
		}
		if len(videos) > 0 {
			out.VideoProviders[provider] = videos
		}
	}
	for _, provider := range SupportedVisionProviders {
		out.VisionProviders[provider] = VisionModelsFor(provider)
	}
	return out
}
