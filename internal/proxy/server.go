package proxy

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"relay-ai/internal/config"
)

type Status struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Addr    string `json:"addr"`
}

type Server struct {
	httpServer *http.Server
	port       int
	mu         sync.RWMutex
	running    bool
	store      *config.Store
	logger     *Logger
	sessions   *SessionStore
}

func New(store *config.Store, db *sql.DB) *Server {
	s := &Server{
		store:    store,
		port:     store.GetPort(),
		logger:   NewLogger(db),
		sessions: NewSessionStore(),
	}
	// Sync slog level with persisted debug mode on startup
	if store.GetDebugMode() {
		logLevel.Set(slog.LevelDebug)
	}
	return s
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.port = s.store.GetPort()
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	handler := newRouter(s.store, s.logger, s.sessions)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		WriteTimeout: 0, // 无写超时，支持长时间流式响应（深度思考）
	}

	go func() {
		slog.Info("proxy starting", "addr", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy error", "error", err)
		}
	}()

	s.running = true
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			return err
		}
	}

	s.running = false
	return nil
}

func (s *Server) Restart() error {
	if err := s.Stop(); err != nil {
		return err
	}
	return s.Start()
}

func (s *Server) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Running: s.running,
		Port:    s.port,
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
	}
}

func (s *Server) GetLogs() []RequestLog {
	return s.logger.GetLogs()
}

func (s *Server) GetLogsWithLimit(limit int) []RequestLog {
	return s.logger.GetLogsWithLimit(limit)
}

func (s *Server) GetProviderUsageSeries(providerID string) []ProviderUsagePoint {
	return s.logger.GetProviderUsageSeries(providerID)
}

func (s *Server) ClearLogs() {
	s.logger.Clear()
}

func (s *Server) GetTotalTokenUsage() (int64, int64, int64) {
	return s.logger.GetTotalTokens()
}

func (s *Server) GetLogsSizeKB() int64 {
	return s.logger.GetSizeKB()
}

// SetDebug enables or disables debug logging by adjusting the slog level.
func (s *Server) SetDebug(enabled bool) {
	if enabled {
		logLevel.Set(slog.LevelDebug)
	} else {
		logLevel.Set(slog.LevelInfo)
	}
}
