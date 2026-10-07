package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/provider/anthropic"
	"github.com/teexue/nexakit/provider/ollama"
	"github.com/teexue/nexakit/provider/openai"
)

// Profile is a resolved provider configuration.
type Profile struct {
	Name         string
	APIStyle     provider.APIStyle
	BaseURL      string
	APIKey       string
	APIVersion   string
	AuthStyle    provider.AuthStyle
	DisplayName  string
	DefaultModel string
	// Models is the user-selected subset available for conversation.
	Models     []string
	ModelsPath string
	Vision     bool
	Thinking   *provider.ThinkingConfig
	// KeepAlive is the Ollama model keep-alive duration (e.g. "5m", "0").
	// Only meaningful for StyleOllama; ignored by other styles.
	KeepAlive string
	// ModelWindows maps model id → context window in tokens, captured when
	// the provider is configured (e.g. Ollama /api/show).
	ModelWindows map[string]int
}

// ProfileEntry is a provider definition held in memory.
type ProfileEntry struct {
	APIStyle     provider.APIStyle
	BaseURL      string
	APIKeyEnv    string
	APIVersion   string
	AuthStyle    provider.AuthStyle
	DefaultModel string
	DisplayName  string
	// Models is the subset of vendor models enabled for conversation.
	// Empty means only DefaultModel is available (legacy providers).
	Models     []string `json:"models,omitempty"`
	ModelsPath string
	Vision     bool
	Thinking   *provider.ThinkingConfig
	KeepAlive  string `json:"keep_alive,omitempty"`
	// ModelWindows maps model id → context window in tokens, captured when
	// the provider is saved so agents can use the real window instead of 128K.
	ModelWindows map[string]int `json:"model_windows,omitempty"`
}

// Catalog holds named provider profiles.
type Catalog struct {
	entries    map[string]ProfileEntry
	credLookup func(string) string
}

// NewCatalog builds a Catalog from in-memory entries.
// An empty map is reported via IsMissingCatalogError.
func NewCatalog(entries map[string]ProfileEntry, credLookup func(string) string) (*Catalog, error) {
	if len(entries) == 0 {
		return nil, &missingCatalogError{path: "db", empty: true}
	}
	for name, entry := range entries {
		if err := entry.validate(); err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
	}
	return &Catalog{entries: entries, credLookup: credLookup}, nil
}

// MissingCatalog reports that a catalog file is absent.
func MissingCatalog(path string) error {
	return &missingCatalogError{path: path}
}

// missingCatalogError indicates the providers file is absent or contains no providers.
type missingCatalogError struct {
	path  string
	empty bool
}

func (e *missingCatalogError) Error() string {
	if e.empty {
		return fmt.Sprintf("providers %q: no providers configured", e.path)
	}
	return fmt.Sprintf("providers %q: not found", e.path)
}

// IsMissingCatalogError reports whether err means the catalog is absent or empty.
func IsMissingCatalogError(err error) bool {
	var m *missingCatalogError
	return errors.As(err, &m)
}

func (e ProfileEntry) validate() error {
	if e.APIStyle == "" {
		return fmt.Errorf("api_style is required")
	}
	if e.APIStyle != provider.StyleAnthropic && e.APIStyle != provider.StyleOpenAI && e.APIStyle != provider.StyleOllama {
		return fmt.Errorf("unsupported api_style %q", e.APIStyle)
	}
	// Local Ollama needs no API key, so api_key_env is optional for that style.
	// When set (e.g. ollama_cloud), it must be a variable name, not the key itself.
	if e.APIKeyEnv == "" && e.APIStyle != provider.StyleOllama {
		return fmt.Errorf("api_key_env is required")
	}
	if e.APIKeyEnv != "" && looksLikeAPIKey(e.APIKeyEnv) {
		return fmt.Errorf("api_key_env must be an environment variable name (e.g. ANTHROPIC_API_KEY), not the key value itself")
	}
	return nil
}

func lookupAPIKey(envName string, credLookup func(string) string) string {
	if v := os.Getenv(envName); v != "" {
		return v
	}
	if credLookup != nil {
		return credLookup(envName)
	}
	return ""
}

func looksLikeAPIKey(s string) bool {
	return strings.HasPrefix(s, "sk-") || strings.HasPrefix(s, "sk_")
}

func (e ProfileEntry) resolve(name string, credLookup func(string) string) (Profile, error) {
	apiKey := lookupAPIKey(e.APIKeyEnv, credLookup)
	// A configured api_key_env means a key is required (e.g. ollama_cloud).
	// An empty api_key_env (local Ollama) means no key is needed.
	if e.APIKeyEnv != "" && apiKey == "" {
		return Profile{}, fmt.Errorf("API key for %q not found (environment variable %s)", e.APIKeyEnv, e.APIKeyEnv)
	}

	vendor, hasVendor := provider.LookupVendor(name)
	baseURL := resolveBaseURL(e, vendor, hasVendor)
	apiVersion := e.APIVersion
	if apiVersion == "" && hasVendor && vendor.APIVersion != "" {
		apiVersion = vendor.APIVersion
	}
	if e.APIStyle == provider.StyleAnthropic && apiVersion == "" {
		apiVersion = provider.DefaultAnthropicVersion
	}
	authStyle := resolveAuthStyle(e, vendor, hasVendor)
	modelsPath := e.ModelsPath
	if modelsPath == "" {
		modelsPath = provider.DefaultModelsPathFor(e.APIStyle)
	}
	displayName := e.DisplayName
	if displayName == "" && hasVendor {
		displayName = vendor.DisplayName
	}

	return Profile{
		Name:         name,
		APIStyle:     e.APIStyle,
		BaseURL:      baseURL,
		APIKey:       apiKey,
		APIVersion:   apiVersion,
		AuthStyle:    authStyle,
		DisplayName:  displayName,
		DefaultModel: e.DefaultModel,
		Models:       EnabledModels(e),
		ModelsPath:   modelsPath,
		Vision:       e.Vision,
		Thinking:     e.Thinking,
		KeepAlive:    e.KeepAlive,
		ModelWindows: e.ModelWindows,
	}, nil
}

func resolveBaseURL(e ProfileEntry, vendor provider.Vendor, hasVendor bool) string {
	if e.BaseURL != "" {
		return e.BaseURL
	}
	if hasVendor {
		if u := vendor.BaseURLFor(e.APIStyle); u != "" {
			return u
		}
	}
	return defaultBaseURLFor(e.APIStyle)
}

func resolveAuthStyle(e ProfileEntry, vendor provider.Vendor, hasVendor bool) provider.AuthStyle {
	if e.AuthStyle != "" {
		return e.AuthStyle
	}
	if hasVendor {
		return vendor.AuthForStyle(e.APIStyle)
	}
	if e.APIStyle == provider.StyleAnthropic {
		return provider.AuthXAPIKey
	}
	return provider.AuthBearer
}

func defaultBaseURLFor(style provider.APIStyle) string {
	return provider.DefaultBaseURLFor(style)
}

// Get returns a resolved provider profile by name.
func (c *Catalog) Get(name string) (Profile, error) {
	entry, ok := c.entries[name]
	if !ok {
		return Profile{}, fmt.Errorf("provider %q not found", name)
	}
	return entry.resolve(name, c.credLookup)
}

// Names returns configured provider names.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	return names
}

// ModelContextWindow returns the context window saved on the provider for
// model. 0 means the provider has no recorded window for that model.
func (c *Catalog) ModelContextWindow(providerName, model string) int {
	if c == nil || providerName == "" || model == "" {
		return 0
	}
	e, ok := c.entries[providerName]
	if !ok {
		return 0
	}
	return e.ModelWindows[model]
}

// MergeModelWindows copies src over dst. Zero or empty keys are ignored.
// The result is nil when empty so callers can omit an empty map.
func MergeModelWindows(dst, src map[string]int) map[string]int {
	if len(dst) == 0 && len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(dst)+len(src))
	for k, v := range dst {
		if v > 0 {
			out[k] = v
		}
	}
	for k, v := range src {
		if v > 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Info returns summary information about a provider (without secrets).
type Info struct {
	Name          string             `json:"name"`
	APIStyle      provider.APIStyle  `json:"api_style"`
	AuthStyle     provider.AuthStyle `json:"auth_style,omitempty"`
	DisplayName   string             `json:"display_name"`
	BaseURL       string             `json:"base_url"`
	DefaultModel  string             `json:"default_model"`
	Models        []string           `json:"models,omitempty"`
	ModelsPath    string             `json:"models_path"`
	Vision        bool               `json:"vision"`
	APIKeyEnv     string             `json:"api_key_env,omitempty"`
	ModelWindows  map[string]int     `json:"model_windows,omitempty"`
	ContextWindow int                `json:"context_window,omitempty"` // default_model's saved window
}

// Entries returns all configured providers as Info (without API keys).
func (c *Catalog) Entries() []Info {
	infos := make([]Info, 0, len(c.entries))
	for name, entry := range c.entries {
		vendor, hasVendor := provider.LookupVendor(name)
		modelsPath := entry.ModelsPath
		if modelsPath == "" {
			modelsPath = provider.DefaultModelsPathFor(entry.APIStyle)
		}
		displayName := entry.DisplayName
		if displayName == "" && hasVendor {
			displayName = vendor.DisplayName
		}
		baseURL := entry.BaseURL
		if baseURL == "" {
			if hasVendor {
				baseURL = vendor.BaseURLFor(entry.APIStyle)
			}
			if baseURL == "" {
				baseURL = defaultBaseURLFor(entry.APIStyle)
			}
		}
		authStyle := entry.AuthStyle
		if authStyle == "" {
			if hasVendor {
				authStyle = vendor.AuthForStyle(entry.APIStyle)
			} else if entry.APIStyle == provider.StyleAnthropic {
				authStyle = provider.AuthXAPIKey
			} else {
				authStyle = provider.AuthBearer
			}
		}
		infos = append(infos, Info{
			Name:          name,
			APIStyle:      entry.APIStyle,
			AuthStyle:     authStyle,
			DisplayName:   displayName,
			BaseURL:       baseURL,
			DefaultModel:  entry.DefaultModel,
			Models:        EnabledModels(entry),
			ModelsPath:    modelsPath,
			Vision:        entry.Vision,
			APIKeyEnv:     entry.APIKeyEnv,
			ModelWindows:  entry.ModelWindows,
			ContextWindow: entry.ModelWindows[entry.DefaultModel],
		})
	}
	return infos
}

// NewProvider creates a Provider from a profile.
func NewProvider(profile Profile) (provider.Provider, error) {
	switch profile.APIStyle {
	case provider.StyleAnthropic:
		return anthropic.New(anthropic.Config{
			APIKey:     profile.APIKey,
			BaseURL:    profile.BaseURL,
			APIVersion: profile.APIVersion,
			AuthStyle:  profile.AuthStyle,
			ModelsPath: profile.ModelsPath,
			Vision:     profile.Vision,
			Vendor:     profile.Name,
		})
	case provider.StyleOpenAI:
		return openai.New(openai.Config{
			APIKey:     profile.APIKey,
			BaseURL:    profile.BaseURL,
			Thinking:   profile.Thinking,
			ModelsPath: profile.ModelsPath,
			Vision:     profile.Vision,
			Vendor:     profile.Name,
		})
	case provider.StyleOllama:
		o, err := ollama.New(ollama.Config{
			APIKey:     profile.APIKey,
			BaseURL:    profile.BaseURL,
			Thinking:   profile.Thinking,
			ModelsPath: profile.ModelsPath,
			Vision:     profile.Vision,
			KeepAlive:  profile.KeepAlive,
		})
		if err != nil {
			return nil, err
		}
		o.SeedContextWindows(profile.ModelWindows)
		return o, nil
	default:
		return nil, fmt.Errorf("unsupported provider api_style %q", profile.APIStyle)
	}
}

// ResolveForAgent returns a provider for the agent's provider name.
func (c *Catalog) ResolveForAgent(providerName string) (provider.Provider, error) {
	profile, err := c.Get(providerName)
	if err != nil {
		return nil, err
	}
	return NewProvider(profile)
}

// ListModels fetches the model catalog for a named provider.
// Requires a valid API key; returns an error if the provider is unknown or
// the implementation does not support model listing.
func (c *Catalog) ListModels(ctx context.Context, name string) ([]provider.ModelInfo, error) {
	profile, err := c.Get(name)
	if err != nil {
		return nil, err
	}
	p, err := NewProvider(ListingProfile(profile))
	if err != nil {
		return nil, err
	}
	lister, ok := p.(provider.ModelLister)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support model listing", name)
	}
	return lister.ListModels(ctx)
}

// ShowModel returns structured metadata for a single model from a configured
// provider. Providers that do not implement ModelDetailer return an error.
func (c *Catalog) ShowModel(ctx context.Context, name, model string) (provider.ModelDetail, error) {
	profile, err := c.Get(name)
	if err != nil {
		return provider.ModelDetail{}, err
	}
	p, err := NewProvider(ListingProfile(profile))
	if err != nil {
		return provider.ModelDetail{}, err
	}
	detailer, ok := p.(provider.ModelDetailer)
	if !ok {
		return provider.ModelDetail{}, fmt.Errorf("provider %q does not support model detail", name)
	}
	return detailer.ShowModel(ctx, model)
}

// ListingProfile returns a Profile tuned for model listing.
// Dual-protocol vendors (moonshot/deepseek/zhipu) only expose /models on their
// OpenAI-compatible endpoint — their Anthropic endpoint does not serve a model
// list. So when the configured style is Anthropic but the vendor also speaks
// OpenAI, we transparently list via the OpenAI endpoint using the same API key.
// Pure-Anthropic vendors (e.g. Anthropic itself) keep their native style.
func ListingProfile(profile Profile) Profile {
	v, ok := provider.LookupVendor(profile.Name)
	if !ok || !v.SupportsStyle(provider.StyleOpenAI) {
		return profile
	}
	if profile.APIStyle == provider.StyleOpenAI {
		return profile
	}
	if profile.BaseURL != "" && profile.BaseURL != v.AnthropicBaseURL {
		return profile
	}
	return Profile{
		Name:       profile.Name,
		APIStyle:   provider.StyleOpenAI,
		BaseURL:    v.OpenAIBaseURL,
		APIKey:     profile.APIKey,
		AuthStyle:  provider.AuthBearer,
		ModelsPath: provider.DefaultModelsPathFor(provider.StyleOpenAI),
		Vision:     profile.Vision,
	}
}
