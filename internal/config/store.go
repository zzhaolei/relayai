package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	// 检查是否已有端口配置
	var port int
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'port'").Scan(&port)
	if err == sql.ErrNoRows {
		// 插入默认端口
		_, err = s.db.Exec("INSERT INTO settings (key, value) VALUES ('port', '18900')")
		if err != nil {
			return fmt.Errorf("初始化端口配置失败: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("查询端口配置失败: %w", err)
	}
	return nil
}

func (s *Store) GetSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	settings := AppSettings{
		Port:      s.getPort(),
		Providers: s.getProviders(),
	}
	return settings
}

func (s *Store) getPort() int {
	var port int
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'port'").Scan(&port)
	if err != nil {
		return 18900
	}
	return port
}

// GetDebugMode returns whether debug logging is enabled.
func (s *Store) GetDebugMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var debug bool
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'debug'").Scan(&debug)
	if err != nil {
		return false
	}
	return debug
}

func (s *Store) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getPort()
}

// GetProxyAuthToken 获取代理级 auth token，不存在则自动生成
func (s *Store) GetProxyAuthToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var token string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'proxy_auth_token'").Scan(&token)
	if err == sql.ErrNoRows {
		token = generateAuthToken()
		s.db.Exec("INSERT INTO settings (key, value) VALUES ('proxy_auth_token', ?)", token)
	}
	return token
}

func (s *Store) SetPort(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('port', ?)", port)
	return err
}

func (s *Store) getProviders() []Provider {
	rows, err := s.db.Query("SELECT id, name, base_url, api_key, auth_token, default_model, model_mappings, cli_types, chat_compat_mode, enabled, created_at, prompt_tokens, completion_tokens, total_tokens, usage_updated_at FROM providers ORDER BY created_at")
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanProviders(rows)
}

func (s *Store) GetProviders() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getProviders()
}

func (s *Store) AddProvider(p Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	modelMappingsJSON, _ := json.Marshal(p.ModelMappings)
	cliTypesJSON, _ := json.Marshal(p.CLITypes)

	_, err := s.db.Exec(
		"INSERT INTO providers (id, name, base_url, api_key, auth_token, default_model, model_mappings, cli_types, chat_compat_mode, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		p.ID, p.Name, p.BaseURL, p.APIKey, p.AuthToken, p.DefaultModel, string(modelMappingsJSON), string(cliTypesJSON), p.ChatCompatMode, p.Enabled, p.CreatedAt,
	)
	return err
}

func (s *Store) UpdateProvider(id string, p Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	modelMappingsJSON, _ := json.Marshal(p.ModelMappings)
	cliTypesJSON, _ := json.Marshal(p.CLITypes)

	_, err := s.db.Exec(
		"UPDATE providers SET name=?, base_url=?, api_key=?, default_model=?, model_mappings=?, cli_types=?, chat_compat_mode=? WHERE id=?",
		p.Name, p.BaseURL, p.APIKey, p.DefaultModel, string(modelMappingsJSON), string(cliTypesJSON), p.ChatCompatMode, id,
	)
	return err
}

func (s *Store) DeleteProvider(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM providers WHERE id = ?", id)
	return err
}

func (s *Store) SetProviderEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE providers SET enabled = ? WHERE id = ?", enabled, id)
	return err
}

func (s *Store) GetProvider(id string) *Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, name, base_url, api_key, auth_token, default_model, model_mappings, cli_types, chat_compat_mode, enabled, created_at, prompt_tokens, completion_tokens, total_tokens, usage_updated_at FROM providers WHERE id = ?", id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	providers := scanProviders(rows)
	if len(providers) == 0 {
		return nil
	}
	return &providers[0]
}

func (s *Store) GetEnabledProviders() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, name, base_url, api_key, auth_token, default_model, model_mappings, cli_types, chat_compat_mode, enabled, created_at, prompt_tokens, completion_tokens, total_tokens, usage_updated_at FROM providers WHERE enabled = 1 ORDER BY created_at")
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanProviders(rows)
}

func (s *Store) GetProviderByAuthToken(token string) *Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, name, base_url, api_key, auth_token, default_model, model_mappings, cli_types, chat_compat_mode, enabled, created_at, prompt_tokens, completion_tokens, total_tokens, usage_updated_at FROM providers WHERE auth_token = ? AND enabled = 1", token)
	if err != nil {
		return nil
	}
	defer rows.Close()
	providers := scanProviders(rows)
	if len(providers) == 0 {
		return nil
	}
	return &providers[0]
}

func scanProviders(rows *sql.Rows) []Provider {
	var providers []Provider
	for rows.Next() {
		var p Provider
		var modelMappingsJSON, cliTypesJSON string
		err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.BaseURL,
			&p.APIKey,
			&p.AuthToken,
			&p.DefaultModel,
			&modelMappingsJSON,
			&cliTypesJSON,
			&p.ChatCompatMode,
			&p.Enabled,
			&p.CreatedAt,
			&p.PromptTokens,
			&p.CompletionTokens,
			&p.TotalTokens,
			&p.UsageUpdatedAt,
		)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(modelMappingsJSON), &p.ModelMappings)
		json.Unmarshal([]byte(cliTypesJSON), &p.CLITypes)
		providers = append(providers, p)
	}
	return providers
}

// SetDebugMode enables or disables debug logging.
func (s *Store) SetDebugMode(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	val := "false"
	if enabled {
		val = "true"
	}
	_, err := s.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('debug_mode', ?)", val)
	return err
}
