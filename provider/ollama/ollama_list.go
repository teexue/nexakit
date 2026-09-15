package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/teexue/nexakit/provider"
)

// ollamaTagsResponse is the shape of GET /api/tags.
type ollamaTagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Details struct {
			ParameterSize string   `json:"parameter_size"`
			Families      []string `json:"families"`
		} `json:"details"`
	} `json:"models"`
}

// ListModels fetches available models from GET /api/tags.
func (o *Ollama) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+o.modelsPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create ollama tags request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama tags request: %w", err)
	}
	// The body is drained below; a close error is irrelevant to the result.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama tags request failed: status %d: %s", resp.StatusCode, string(body))
	}
	var out ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode ollama tags: %w", err)
	}
	models := make([]provider.ModelInfo, 0, len(out.Models))
	for _, m := range out.Models {
		mi := provider.ModelInfo{ID: m.Name}
		for _, fam := range m.Details.Families {
			if fam == "clip" || fam == "vision" {
				mi.Vision = true
			}
		}
		models = append(models, mi)
	}
	return models, nil
}
