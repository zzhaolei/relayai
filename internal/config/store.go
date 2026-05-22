package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	settings *AppSettings
	filePath string
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".one-switch")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		filePath: filepath.Join(dir, "config.json"),
		settings: DefaultSettings(),
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, s.settings)
}

func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.settings, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *Store) GetSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.settings
}

func (s *Store) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.Port
}

func (s *Store) SetPort(port int) error {
	s.mu.Lock()
	s.settings.Port = port
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) GetProviders() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.Providers
}

func (s *Store) AddProvider(p Provider) error {
	s.mu.Lock()
	s.settings.Providers = append(s.settings.Providers, p)
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) UpdateProvider(id string, p Provider) error {
	s.mu.Lock()
	for i, existing := range s.settings.Providers {
		if existing.ID == id {
			p.ID = id
			p.CreatedAt = existing.CreatedAt
			p.Enabled = existing.Enabled
			s.settings.Providers[i] = p
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) DeleteProvider(id string) error {
	s.mu.Lock()
	for i, p := range s.settings.Providers {
		if p.ID == id {
			s.settings.Providers = append(s.settings.Providers[:i], s.settings.Providers[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) SetProviderEnabled(id string, enabled bool) error {
	s.mu.Lock()
	for i, p := range s.settings.Providers {
		if p.ID == id {
			s.settings.Providers[i].Enabled = enabled
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

// GetEnabledProviders returns all providers that are enabled.
func (s *Store) GetEnabledProviders() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Provider
	for _, p := range s.settings.Providers {
		if p.Enabled {
			result = append(result, p)
		}
	}
	return result
}
