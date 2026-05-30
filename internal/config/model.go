package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"time"
)

type ModelMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Provider struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	BaseURL          string         `json:"base_url"`
	APIKey           string         `json:"api_key"`
	AuthToken        string         `json:"auth_token"`
	DefaultModel     string         `json:"default_model"`
	ModelMappings    []ModelMapping `json:"model_mappings"`
	CLITypes         []string       `json:"cli_types"`
	ChatCompatMode   bool           `json:"chat_compat_mode"`
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

func NewProvider(name, baseURL, apiKey string) Provider {
	return Provider{
		ID:        generateID(),
		Name:      name,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		AuthToken: generateAuthToken(),
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}
}

// generateAuthToken 生成本地密钥 sk-local-<40位hex>
func generateAuthToken() string {
	b := make([]byte, 20)
	rand.Read(b)
	h := sha256.Sum256(b)
	return "sk-local-" + hex.EncodeToString(h[:])[:40]
}

func generateID() string {
	return time.Now().Format("20060102150405") + randomSuffix()
}

func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			b[i] = chars[i%len(chars)]
			continue
		}
		b[i] = chars[n.Int64()]
	}
	return string(b)
}
