package cli

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
)

const (
	claudeDir    = ".claude"
	claudeConfig = "settings.json"
)

type ClaudeSettings struct {
	Extra map[string]any    `json:"-"`
	Env   map[string]string `json:"env,omitempty"`
}

func (s *ClaudeSettings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Extra = make(map[string]any)
	for k, v := range raw {
		if k == "env" {
			json.Unmarshal(v, &s.Env)
		} else {
			var val any
			json.Unmarshal(v, &val)
			s.Extra[k] = val
		}
	}
	return nil
}

func (s *ClaudeSettings) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	maps.Copy(m, s.Extra)
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
