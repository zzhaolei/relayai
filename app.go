package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"unsafe"

	"relay-ai/internal/cli"
	"relay-ai/internal/config"
	"relay-ai/internal/database"
	"relay-ai/internal/native"
	"relay-ai/internal/proxy"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var providerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type App struct {
	store *config.Store
	proxy *proxy.Server
	db    *database.DB
	win   application.Window
}

type ProxyStatus struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Addr    string `json:"addr"`
}

func NewApp() *App {
	db, err := database.New()
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	store, err := config.NewStore(db.Conn())
	if err != nil {
		log.Fatalf("failed to init config store: %v", err)
	}

	return &App{
		store: store,
		proxy: proxy.New(store, db.Conn()),
		db:    db,
	}
}

func (a *App) setWindow(win application.Window) {
	a.win = win
}

func (a *App) initProxy() error {
	return a.proxy.Start()
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
	providers := a.store.GetProviders()
	for i := range providers {
		maskAPIKey(&providers[i])
	}
	return providers
}

func (a *App) ProviderCreate(name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string, chatCompatMode bool) (config.Provider, error) {
	if !providerNamePattern.MatchString(name) {
		return config.Provider{}, fmt.Errorf("provider name only supports English letters, numbers, underscores, and hyphens")
	}
	p := config.NewProvider(name, baseURL, apiKey)
	p.DefaultModel = defaultModel
	p.ModelMappings = modelMappings
	p.CLITypes = cliTypes
	p.ChatCompatMode = chatCompatMode
	if err := a.store.AddProvider(p); err != nil {
		return config.Provider{}, err
	}
	maskAPIKey(&p)
	return p, nil
}

func maskAPIKey(p *config.Provider) {
	p.APIKey = p.AuthToken
}

func (a *App) ProviderUpdate(id, name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string, chatCompatMode bool) error {
	if !providerNamePattern.MatchString(name) {
		return fmt.Errorf("provider name only supports English letters, numbers, underscores, and hyphens")
	}
	// 如果提交的是本地 key，保持原有真实 key 不变
	if strings.HasPrefix(apiKey, "sk-local-") {
		if existing := a.store.GetProvider(id); existing != nil {
			apiKey = existing.APIKey
		}
	}
	p := config.Provider{
		Name:           name,
		BaseURL:        baseURL,
		APIKey:         apiKey,
		DefaultModel:   defaultModel,
		ModelMappings:  modelMappings,
		CLITypes:       cliTypes,
		ChatCompatMode: chatCompatMode,
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

func (a *App) WriteCLIConfig(cliType string) error {
	enabled := a.store.GetEnabledProviders()
	if len(enabled) == 0 {
		return fmt.Errorf("no enabled providers")
	}

	// 选择第一个支持该 CLI 类型的 provider
	var provider *config.Provider
	for _, p := range enabled {
		if len(p.CLITypes) == 0 || containsStr(p.CLITypes, cliType) {
			provider = &p
			break
		}
	}
	if provider == nil {
		return fmt.Errorf("no provider configured for %s", cliType)
	}

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", a.store.GetPort())
	proxyBaseURL := fmt.Sprintf("http://%s", proxyAddr)

	switch cliType {
	case "claude":
		return cli.EnableClaudeProvider(proxyBaseURL+"/anthropic", provider.AuthToken)
	case "codex":
		return cli.EnableCodexProvider(proxyBaseURL+"/openai", provider.AuthToken)
	default:
		return fmt.Errorf("unknown cli type: %s", cliType)
	}
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

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

func (a *App) GetProviderUsageStats() []proxy.ProviderUsageStats {
	return a.proxy.GetProviderUsageStats()
}

func (a *App) GetProviderUsageSeries(providerID string) []proxy.ProviderUsagePoint {
	return a.proxy.GetProviderUsageSeries(providerID)
}

func (a *App) ClearProxyLogs() {
	a.proxy.ClearLogs()
}

func (a *App) GetProxyLogsSizeKB() int64 {
	return a.proxy.GetLogsSizeKB()
}

// --- Settings ---

func (a *App) SettingsGet() config.AppSettings {
	return a.store.GetSettings()
}

func (a *App) SettingsUpdatePort(port int) error {
	return a.store.SetPort(port)
}

// --- Appearance ---

func (a *App) SetAppearanceMode(mode string) {
	var hwnd unsafe.Pointer
	if a.win != nil {
		hwnd = a.win.NativeWindow()
	}
	native.SetWindowAppearance(hwnd, mode)
}
