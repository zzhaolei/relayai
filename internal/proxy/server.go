package proxy

import (
	"database/sql"
	"fmt"
	"log"
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
}

func New(store *config.Store, db *sql.DB) *Server {
	return &Server{
		store:  store,
		port:   store.GetPort(),
		logger: NewLogger(db),
	}
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.port = s.store.GetPort()
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	handler := newRouter(s.store, s.logger)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: handler,
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

func (s *Server) ClearLogs() {
	s.logger.Clear()
}

func (s *Server) GetLogsSizeKB() int64 {
	return s.logger.GetSizeKB()
}
