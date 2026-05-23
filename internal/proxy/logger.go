package proxy

import (
	"fmt"
	"sync"
	"time"
)

const (
	maxLogEntries = 500
	logTTL        = 24 * time.Hour
)

type RequestLog struct {
	ID           string `json:"id"`
	Time         int64  `json:"time"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	CLIType      string `json:"cli_type"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	StatusCode   int    `json:"status_code"`
	Duration     int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
}

type Logger struct {
	mu   sync.RWMutex
	logs []RequestLog
	seq  int64
}

func NewLogger() *Logger {
	return &Logger{
		logs: make([]RequestLog, 0, maxLogEntries),
	}
}

func (l *Logger) Add(entry RequestLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry.ID = fmt.Sprintf("%d", l.seq)
	entry.Time = time.Now().UnixMilli()

	// Evict logs older than 24h
	cutoff := time.Now().Add(-logTTL).UnixMilli()
	for len(l.logs) > 0 && l.logs[0].Time < cutoff {
		l.logs = l.logs[1:]
	}

	if len(l.logs) >= maxLogEntries {
		l.logs = l.logs[1:]
	}
	l.logs = append(l.logs, entry)
}

func (l *Logger) GetLogs() []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]RequestLog, len(l.logs))
	copy(result, l.logs)
	// Reverse to show newest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = l.logs[:0]
}
