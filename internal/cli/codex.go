package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	codexDir    = ".codex"
	codexConfig = "config.toml"
)

func codexConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, codexDir, codexConfig), nil
}

func codexAuthPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, codexDir, "auth.json"), nil
}

func codexEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, codexDir, "relayai_env.sh"), nil
}

func ReadCodexConfig() (map[string]interface{}, error) {
	path, err := codexConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func WriteCodexConfig(m map[string]interface{}) error {
	path, err := codexConfigPath()
	if err != nil {
		return err
	}
	backupFile(path)
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetTablesInline(false)
	if err := enc.Encode(m); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func EnableCodexProvider(baseURL, apiKey string) error {
	m, err := ReadCodexConfig()
	if err != nil {
		if os.IsNotExist(err) {
			m = make(map[string]interface{})
		} else {
			return err
		}
	}

	m["model_provider"] = "relayai"

	providers, _ := m["model_providers"].(map[string]interface{})
	if providers == nil {
		providers = make(map[string]interface{})
	}

	providers["relayai"] = map[string]interface{}{
		"name":                 "RelayAI",
		"base_url":             strings.TrimRight(baseURL, "/"),
		"requires_openai_auth": true,
		"wire_api":             "responses",
	}

	m["model_providers"] = providers

	// Set recommended Codex options
	m["model_reasoning_effort"] = "xhigh"

	features, _ := m["features"].(map[string]interface{})
	if features == nil {
		features = make(map[string]interface{})
	}
	features["goals"] = true
	m["features"] = features

	if err := WriteCodexConfig(m); err != nil {
		return err
	}

	// Write auth.json
	authPath, _ := codexAuthPath()
	authContent := fmt.Sprintf("{\n  \"auth_mode\": \"apikey\",\n  \"OPENAI_API_KEY\": \"%s\"\n}\n", apiKey)
	if err := os.WriteFile(authPath, []byte(authContent), 0600); err != nil {
		return err
	}

	// Write env file for OPENAI_API_KEY
	envPath, _ := codexEnvPath()
	envContent := fmt.Sprintf("# RelayAI auto-generated\nexport OPENAI_API_KEY=\"%s\"\n", apiKey)
	return os.WriteFile(envPath, []byte(envContent), 0644)
}

func DisableCodexProvider() error {
	m, err := ReadCodexConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if m["model_provider"] == "relayai" {
		delete(m, "model_provider")
	}

	if providers, ok := m["model_providers"].(map[string]interface{}); ok {
		delete(providers, "relayai")
		m["model_providers"] = providers
	}

	// Remove env file and auth.json
	envPath, _ := codexEnvPath()
	os.Remove(envPath)
	authPath, _ := codexAuthPath()
	os.Remove(authPath)

	return WriteCodexConfig(m)
}

func IsCodexEnabled(proxyAddr string) bool {
	m, err := ReadCodexConfig()
	if err != nil {
		return false
	}
	if m["model_provider"] != "relayai" {
		return false
	}
	providers, ok := m["model_providers"].(map[string]interface{})
	if !ok {
		return false
	}
	relayai, ok := providers["relayai"].(map[string]interface{})
	if !ok {
		return false
	}
	expected := fmt.Sprintf("http://%s/openai", proxyAddr)
	return relayai["base_url"] == expected
}

func GetCodexEnvFilePath() string {
	path, _ := codexEnvPath()
	return path
}
