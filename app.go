package main

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"
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

// capitalizeCLI returns a display-friendly name for CLI types (e.g. "claude" -> "Claude").
func capitalizeCLI(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type App struct {
	store *config.Store
	proxy *proxy.Server
	db    *database.DB
	win   application.Window
}

type ProxyStatus struct {
	Running        bool   `json:"running"`
	Port           int    `json:"port"`
	Addr           string `json:"addr"`
	ProxyAuthToken string `json:"proxy_auth_token"`
}

func NewApp() *App {
	db, err := database.New()
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}

	store, err := config.NewStore(db.Conn())
	if err != nil {
		slog.Error("failed to init config store", "error", err)
		os.Exit(1)
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
		Running:        s.Running,
		Port:           s.Port,
		Addr:           s.Addr,
		ProxyAuthToken: a.store.GetProxyAuthToken(),
	}
}

// --- Provider CRUD ---

func (a *App) ProviderList() []config.Provider {
	return a.store.GetProviders()
}

// checkDuplicateProviderName checks if a provider with the same name already exists.
// Returns an error if a conflict is found.
// When editing (updateID != ""), that provider is excluded from the check.
func (a *App) checkDuplicateProviderName(name string, updateID string) error {
	for _, existing := range a.store.GetProviders() {
		if existing.ID == updateID {
			continue
		}
		if existing.Name == name {
			return fmt.Errorf("提供商名称「%s」已存在，请更换名称", name)
		}
	}
	return nil
}

func (a *App) ProviderCreate(name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string, chatCompatMode bool) (config.Provider, error) {
	if !providerNamePattern.MatchString(name) {
		return config.Provider{}, fmt.Errorf("provider name only supports English letters, numbers, underscores, and hyphens")
	}
	if err := a.checkDuplicateProviderName(name, ""); err != nil {
		return config.Provider{}, err
	}
	p := config.NewProvider(name, baseURL, apiKey)
	p.DefaultModel = defaultModel
	p.ModelMappings = modelMappings
	p.CLITypes = cliTypes
	p.ChatCompatMode = chatCompatMode
	if err := a.store.AddProvider(p); err != nil {
		return config.Provider{}, err
	}
	return p, nil
}

func (a *App) ProviderUpdate(id, name, baseURL, apiKey string, defaultModel string, modelMappings []config.ModelMapping, cliTypes []string, chatCompatMode bool) error {
	if !providerNamePattern.MatchString(name) {
		return fmt.Errorf("provider name only supports English letters, numbers, underscores, and hyphens")
	}
	if err := a.checkDuplicateProviderName(name, id); err != nil {
		return err
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
		return fmt.Errorf("没有可用的提供商")
	}

	// 选择第一个支持该 CLI 类型的 provider
	var provider *config.Provider
	for _, p := range enabled {
		if len(p.CLITypes) == 0 || slices.Contains(p.CLITypes, cliType) {
			provider = &p
			break
		}
	}
	if provider == nil {
		return fmt.Errorf("未配置 %s 平台的提供商", capitalizeCLI(cliType))
	}

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", a.store.GetPort())
	proxyBaseURL := fmt.Sprintf("http://%s", proxyAddr)

	proxyToken := a.store.GetProxyAuthToken()
	switch cliType {
	case "claude":
		return cli.EnableClaudeProvider(proxyBaseURL+"/anthropic", proxyToken)
	case "codex":
		return cli.EnableCodexProvider(proxyBaseURL+"/openai", proxyToken)
	default:
		return fmt.Errorf("unknown cli type: %s", cliType)
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

// --- Appearance ---

func (a *App) SetAppearanceMode(mode string) {
	var hwnd unsafe.Pointer
	if a.win != nil {
		hwnd = a.win.NativeWindow()
	}
	native.SetWindowAppearance(hwnd, mode)
}

// --- Debug Mode ---

func (a *App) GetDebugMode() bool {
	return a.store.GetDebugMode()
}

func (a *App) SetDebugMode(enabled bool) error {
	if err := a.store.SetDebugMode(enabled); err != nil {
		return err
	}
	a.proxy.SetDebug(enabled)
	return nil
}

// --- Combined log fetch (single IPC call) ---

type ProxyLogData struct {
	Logs      []proxy.RequestLog `json:"logs"`
	SizeKB    int64              `json:"sizeKB"`
	TotalUsed int                `json:"totalUsed"`
}

func (a *App) GetProxyLogData() ProxyLogData {
	logs := a.proxy.GetLogs()
	totalUsed := 0
	for _, l := range logs {
		totalUsed += l.TotalTokens
	}
	return ProxyLogData{
		Logs:      logs,
		SizeKB:    a.proxy.GetLogsSizeKB(),
		TotalUsed: totalUsed,
	}
}

// --- Limited log fetch (for initial load) ---

func (a *App) GetProxyLogDataWithLimit(limit int) ProxyLogData {
	logs := a.proxy.GetLogsWithLimit(limit)
	totalUsed := 0
	for _, l := range logs {
		totalUsed += l.TotalTokens
	}
	return ProxyLogData{
		Logs:      logs,
		SizeKB:    a.proxy.GetLogsSizeKB(),
		TotalUsed: totalUsed,
	}
}

// --- Incremental log fetch ---

type ProxyLogDataSince struct {
	Logs      []proxy.RequestLog `json:"logs"`
	SizeKB    int64              `json:"sizeKB"`
	TotalUsed int                `json:"totalUsed"`
}

func (a *App) GetProxyLogDataSince(lastID string) ProxyLogDataSince {
	logs := a.proxy.GetLogsSince(lastID)
	totalUsed := 0
	for _, l := range logs {
		totalUsed += l.TotalTokens
	}
	return ProxyLogDataSince{
		Logs:      logs,
		SizeKB:    a.proxy.GetLogsSizeKB(),
		TotalUsed: totalUsed,
	}
}

// --- Date range log fetch ---

func (a *App) GetProxyLogDataByTimeRange(from int64, to int64) ProxyLogDataSince {
	logs := a.proxy.GetLogsByTimeRange(from, to)
	totalUsed := 0
	for _, l := range logs {
		totalUsed += l.TotalTokens
	}
	return ProxyLogDataSince{
		Logs:      logs,
		SizeKB:    a.proxy.GetLogsSizeKB(),
		TotalUsed: totalUsed,
	}
}
