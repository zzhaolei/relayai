package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	geminiDir    = ".gemini"
	geminiConfig = "settings.json"
)

func geminiConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, geminiDir, geminiConfig), nil
}

func ReadGeminiSettings() (map[string]interface{}, error) {
	path, err := geminiConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func WriteGeminiSettings(m map[string]interface{}) error {
	path, err := geminiConfigPath()
	if err != nil {
		return err
	}
	backupFile(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func EnableGeminiProvider(baseURL, apiKey string) error {
	m, err := ReadGeminiSettings()
	if err != nil {
		if os.IsNotExist(err) {
			m = make(map[string]interface{})
		} else {
			return err
		}
	}

	m["apiKey"] = apiKey
	m["apiBaseUrl"] = baseURL

	return WriteGeminiSettings(m)
}

func DisableGeminiProvider() error {
	m, err := ReadGeminiSettings()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	delete(m, "apiKey")
	delete(m, "apiBaseUrl")

	return WriteGeminiSettings(m)
}

func IsGeminiEnabled(proxyAddr string) bool {
	m, err := ReadGeminiSettings()
	if err != nil {
		return false
	}
	expected := fmt.Sprintf("http://%s/gemini", proxyAddr)
	return m["apiBaseUrl"] == expected
}
