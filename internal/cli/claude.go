package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	claudeDir    = ".claude"
	claudeConfig = "settings.json"
)

type ClaudeSettings struct {
	Extra map[string]interface{} `json:"-"`
	Env   map[string]string      `json:"env,omitempty"`
}

func (s *ClaudeSettings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Extra = make(map[string]interface{})
	for k, v := range raw {
		if k == "env" {
			json.Unmarshal(v, &s.Env)
		} else {
			var val interface{}
			json.Unmarshal(v, &val)
			s.Extra[k] = val
		}
	}
	return nil
}

func (s *ClaudeSettings) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	for k, v := range s.Extra {
		m[k] = v
	}
	if s.Env != nil {
		m["env"] = s.Env
	}
	return json.MarshalIndent(m, "", "  ")
}

func claudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeDir, claudeConfig), nil
}

func ReadClaudeSettings() (*ClaudeSettings, error) {
	path, err := claudeConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s ClaudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Env == nil {
		s.Env = make(map[string]string)
	}
	return &s, nil
}

func WriteClaudeSettings(s *ClaudeSettings) error {
	path, err := claudeConfigPath()
	if err != nil {
		return err
	}
	backupFile(path)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func EnableClaudeProvider(baseURL, apiKey string) error {
	s, err := ReadClaudeSettings()
	if err != nil {
		if os.IsNotExist(err) {
			s = &ClaudeSettings{Env: make(map[string]string)}
		} else {
			return err
		}
	}
	if s.Env == nil {
		s.Env = make(map[string]string)
	}
	s.Env["ANTHROPIC_BASE_URL"] = baseURL
	s.Env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	return WriteClaudeSettings(s)
}

func DisableClaudeProvider() error {
	s, err := ReadClaudeSettings()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	delete(s.Env, "ANTHROPIC_BASE_URL")
	delete(s.Env, "ANTHROPIC_AUTH_TOKEN")
	return WriteClaudeSettings(s)
}

func IsClaudeEnabled(proxyAddr string) bool {
	s, err := ReadClaudeSettings()
	if err != nil {
		return false
	}
	return s.Env["ANTHROPIC_BASE_URL"] == fmt.Sprintf("http://%s/anthropic", proxyAddr)
}
