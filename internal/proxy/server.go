package proxy

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

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
	debug      atomic.Bool
}

func New(store *config.Store, db *sql.DB) *Server {
	s := &Server{
		store:    store,
		port:     store.GetPort(),
		logger:   NewLogger(db),
		sessions: NewSessionStore(),
	}
	s.debug.Store(store.GetDebugMode())
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
	handler := newRouter(s.store, s.logger, s.sessions, &s.debug)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		WriteTimeout: 0, // 无写超时，支持长时间流式响应（深度思考）
	}

	go func() {
		log.Printf("proxy starting on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("proxy error: %v", err)
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

func (s *Server) GetProviderUsageStats() []ProviderUsageStats {
	return s.logger.GetProviderUsageStats()
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

// SetDebug enables or disables debug logging.
func (s *Server) SetDebug(enabled bool) {
	s.debug.Store(enabled)
}

// IsDebug returns whether debug logging is enabled.
func (s *Server) IsDebug() bool {
	return s.debug.Load()
}
