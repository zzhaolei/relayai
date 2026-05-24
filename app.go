package main

import (
	"context"
	"fmt"
	"log"

	"relay-ai/internal/cli"
	"relay-ai/internal/config"
	"relay-ai/internal/proxy"
)

type App struct {
	ctx   context.Context
	store *config.Store
	proxy *proxy.Server
}

type ProxyStatus struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Addr    string `json:"addr"`
}

func NewApp() *App {
	store, err := config.NewStore()
	if err != nil {
		log.Fatalf("failed to init config store: %v", err)
	}
	return &App{
		store: store,
		proxy: proxy.New(store),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.proxy.Start(); err != nil {
		log.Printf("failed to start proxy: %v", err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.proxy.Stop()
}

// --- Proxy lifecycle ---

func (a *App) ProxyStart() error {
	return a.proxy.Start()
}

func (a *App) ProxyStop() error {
	return a.proxy.Stop()
}

func (a *App) ProxyRestart() error {
	return a.proxy.Restart()
}

func (a *App) ProxyStatus() ProxyStatus {
	s := a.proxy.Status()
	return ProxyStatus{
		Running: s.Running,
		Port:    s.Port,
		Addr:    s.Addr,
	}
}

// --- Provider CRUD ---

func (a *App) ProviderList() []config.Provider {
	return a.store.GetProviders()
}

func (a *App) ProviderCreate(name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string) (config.Provider, error) {
	p := config.NewProvider(name, baseURL, apiKey)
	p.DefaultModel = defaultModel
	p.ModelMappings = modelMappings
	p.CLITypes = cliTypes
	if err := a.store.AddProvider(p); err != nil {
		return config.Provider{}, err
	}
	return p, nil
}

func (a *App) ProviderUpdate(id, name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string) error {
	p := config.Provider{
		Name:          name,
		BaseURL:       baseURL,
		APIKey:        apiKey,
		DefaultModel:  defaultModel,
		ModelMappings: modelMappings,
		CLITypes:      cliTypes,
	}
	return a.store.UpdateProvider(id, p)
}

func (a *App) ProviderDelete(id string) error {
	return a.store.DeleteProvider(id)
}

func (a *App) ProviderSetEnabled(id string, enabled bool) error {
	return a.store.SetProviderEnabled(id, enabled)
}

// --- CLI Config Writing ---

// WriteCLIConfig writes the proxy URL and key into the specified CLI's config file.
func (a *App) WriteCLIConfig(cliType string) error {
	enabled := a.store.GetEnabledProviders()
	if len(enabled) == 0 {
		return fmt.Errorf("no enabled providers")
	}

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", a.store.GetPort())
	proxyBaseURL := fmt.Sprintf("http://%s", proxyAddr)

	// Use the first enabled provider's API key for the CLI config
	apiKey := enabled[0].APIKey

	switch cliType {
	case "claude":
		return cli.EnableClaudeProvider(proxyBaseURL+"/anthropic", apiKey)
	case "codex":
		return cli.EnableCodexProvider(proxyBaseURL+"/openai", apiKey)
	default:
		return fmt.Errorf("unknown cli type: %s", cliType)
	}
}

// GetCLIConfigStatus checks which CLIs are currently pointing to our proxy.
func (a *App) GetCLIConfigStatus() map[string]bool {
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", a.store.GetPort())
	return map[string]bool{
		"claude": cli.IsClaudeEnabled(proxyAddr),
		"codex":  cli.IsCodexEnabled(proxyAddr),
	}
}

// --- Logs ---

func (a *App) GetProxyLogs() []proxy.RequestLog {
	return a.proxy.GetLogs()
}

func (a *App) ClearProxyLogs() {
	a.proxy.ClearLogs()
}

// --- Settings ---

func (a *App) SettingsGet() config.AppSettings {
	return a.store.GetSettings()
}

func (a *App) SettingsUpdatePort(port int) error {
	return a.store.SetPort(port)
}
