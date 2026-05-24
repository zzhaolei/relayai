package proxy

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

const (
	logTTL = 7 * 24 * time.Hour // 保留 7 天日志
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
	mu  sync.RWMutex
	db  *sql.DB
	seq int64
}

func NewLogger(db *sql.DB) *Logger {
	l := &Logger{db: db}
	l.init()
	return l
}

func (l *Logger) init() {
	// 启动时清理过期日志
	cutoff := time.Now().Add(-logTTL).UnixMilli()
	l.db.Exec("DELETE FROM request_logs WHERE time < ?", cutoff)

	// 获取当前最大序号
	var maxID sql.NullInt64
	l.db.QueryRow("SELECT MAX(CAST(id AS INTEGER)) FROM request_logs").Scan(&maxID)
	if maxID.Valid {
		l.seq = maxID.Int64
	}
}

func (l *Logger) Add(entry RequestLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry.ID = fmt.Sprintf("%d", l.seq)
	entry.Time = time.Now().UnixMilli()

	_, err := l.db.Exec(
		"INSERT INTO request_logs (id, time, method, path, cli_type, provider, model, status_code, duration_ms, error, response_body) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		entry.ID, entry.Time, entry.Method, entry.Path, entry.CLIType, entry.Provider, entry.Model, entry.StatusCode, entry.Duration, entry.Error, entry.ResponseBody,
	)
	if err != nil {
		// 日志写入失败不影响主流程
		fmt.Printf("写入日志失败: %v\n", err)
	}
}

func (l *Logger) GetLogs() []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query("SELECT id, time, method, path, cli_type, provider, model, status_code, duration_ms, error, response_body FROM request_logs ORDER BY time DESC LIMIT 500")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.CLIType, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
}

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.db.Exec("DELETE FROM request_logs")
}
