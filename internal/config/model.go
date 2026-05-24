package config

import "time"

type ModelMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Provider struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	BaseURL          string         `json:"base_url"`
	APIKey           string         `json:"api_key"`
	DefaultModel     string         `json:"default_model"`
	ModelMappings    []ModelMapping `json:"model_mappings"`
	CLITypes         []string       `json:"cli_types"`
	Enabled          bool           `json:"enabled"`
	CreatedAt        int64          `json:"created_at"`
	PromptTokens     int64          `json:"prompt_tokens"`
	CompletionTokens int64          `json:"completion_tokens"`
	TotalTokens      int64          `json:"total_tokens"`
	UsageUpdatedAt   int64          `json:"usage_updated_at"`
}

type AppSettings struct {
	Port      int        `json:"port"`
	Providers []Provider `json:"providers"`
}

func DefaultSettings() *AppSettings {
	return &AppSettings{
		Port:      18900,
		Providers: []Provider{},
	}
}

func NewProvider(name, baseURL, apiKey string) Provider {
	return Provider{
		ID:        generateID(),
		Name:      name,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}
}

func generateID() string {
	return time.Now().Format("20060102150405") + randomSuffix()
}

func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(b)
}
