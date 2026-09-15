package provider

import "strings"

// CatalogModelRow is the per-model entry returned by OpenAI-compatible and
// Anthropic-compatible model-list endpoints. The two wire formats overlap
// enough that a single decoding shape suffices; the fields are optional since
// each vendor fills a different subset.
type CatalogModelRow struct {
	ID             string              `json:"id"`
	Context        int                 `json:"context_length,omitempty"`
	Vision         *bool               `json:"vision,omitempty"`
	SupportsVision *bool               `json:"supports_vision,omitempty"`
	Capabilities   []string            `json:"capabilities,omitempty"`
	Modalities     []string            `json:"modalities,omitempty"`
	Architecture   CatalogArchitecture `json:"architecture"`
}

// CatalogArchitecture is the architecture sub-object of a catalog model row.
type CatalogArchitecture struct {
	Modality        string   `json:"modality"`
	InputModalities []string `json:"input_modalities"`
}

// ToModelInfo converts a wire row to a ModelInfo.
func (r CatalogModelRow) ToModelInfo() ModelInfo {
	return ModelInfo{
		ID:            r.ID,
		ContextWindow: r.Context,
		Vision:        r.HasVision(),
	}
}

// HasVision reports whether any vision signal is present in the row.
func (r CatalogModelRow) HasVision() bool {
	if r.Vision != nil && *r.Vision {
		return true
	}
	if r.SupportsVision != nil && *r.SupportsVision {
		return true
	}
	if tokensIncludeVision(r.Capabilities) || tokensIncludeVision(r.Modalities) {
		return true
	}
	if tokensIncludeVision(r.Architecture.InputModalities) {
		return true
	}
	return strings.Contains(strings.ToLower(r.Architecture.Modality), "image")
}

func tokensIncludeVision(vals []string) bool {
	for _, v := range vals {
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "vision" || s == "image" {
			return true
		}
	}
	return false
}
