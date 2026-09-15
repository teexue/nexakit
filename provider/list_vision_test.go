package provider

import "testing"

func TestCatalogModelRowHasVision(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		row  CatalogModelRow
		want bool
	}{
		{name: "plain id", row: CatalogModelRow{ID: "gpt-4o"}, want: false},
		{name: "vision true", row: CatalogModelRow{Vision: &yes}, want: true},
		{name: "supports_vision", row: CatalogModelRow{SupportsVision: &yes}, want: true},
		{
			name: "capabilities vision",
			row:  CatalogModelRow{Capabilities: []string{"completion", "vision"}},
			want: true,
		},
		{
			name: "openrouter input_modalities",
			row: CatalogModelRow{Architecture: CatalogArchitecture{
				InputModalities: []string{"text", "image"},
			}},
			want: true,
		},
		{
			name: "openrouter modality string",
			row: CatalogModelRow{Architecture: CatalogArchitecture{
				Modality: "text+image->text",
			}},
			want: true,
		},
		{
			name: "text only",
			row: CatalogModelRow{Architecture: CatalogArchitecture{
				InputModalities: []string{"text"},
				Modality:        "text->text",
			}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.row.ToModelInfo().Vision
			if got != tc.want {
				t.Fatalf("vision = %v, want %v", got, tc.want)
			}
		})
	}
}
