package proxy

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	usageRetention = 12 * time.Hour // 用量曲线保留 12 小时
	maxLogCount    = 10000          // 日志最多保留 10000 条
)

type RequestLog struct {
	ID               string `json:"id"`
	Time             int64  `json:"time"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	UpstreamURL      string `json:"upstream_url,omitempty"`
	CLIType          string `json:"cli_type"`
	ProviderID       string `json:"provider_id,omitempty"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	StatusCode       int    `json:"status_code"`
	Duration         int64  `json:"duration_ms"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Error            string `json:"error,omitempty"`
	ResponseBody     string `json:"response_body,omitempty"`
}

type ProviderUsageStats struct {
	ProviderID       string `json:"provider_id"`
	Provider         string `json:"provider"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	UpdatedAt        int64  `json:"updated_at"`
}

type ProviderUsagePoint struct {
	Time             int64 `json:"time"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
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
	// 获取当前最大序号（必须同步，保证后续 ID 不冲突）
	var maxID sql.NullInt64
	l.db.QueryRow("SELECT MAX(CAST(id AS INTEGER)) FROM request_logs").Scan(&maxID)
	if maxID.Valid {
		l.seq = maxID.Int64
	}

	// 清理放到后台执行，不阻塞启动
	go l.cleanupLoop()
}

// cleanupLoop runs periodic cleanup in the background.
// Removes old usage points (>12h) and excess logs (>1000 or >12h).
func (l *Logger) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		// 删除超过 12 小时的用量曲线
		cutoff := time.Now().Add(-usageRetention).UnixMilli()
		if _, err := l.db.Exec("DELETE FROM provider_usage_points WHERE bucket_start < ?", cutoff); err != nil {
			slog.Error("failed to cleanup old usage points", "error", err)
		}

		// 删除超过 12 小时的日志
		logCutoff := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
		if _, err := l.db.Exec("DELETE FROM request_logs WHERE time < ?", logCutoff); err != nil {
			slog.Error("failed to cleanup old request logs by time", "error", err)
		}

		// 删除超出条数上限的日志
		if _, err := l.db.Exec(
			"DELETE FROM request_logs WHERE id NOT IN (SELECT id FROM request_logs ORDER BY time DESC LIMIT ?)",
			maxLogCount,
		); err != nil {
			slog.Error("failed to cleanup excess request logs", "error", err)
		}
	}
}

func (l *Logger) Add(entry RequestLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry.ID = fmt.Sprintf("%d", l.seq)
	entry.Time = time.Now().UnixMilli()

	_, err := l.db.Exec(
		"INSERT INTO request_logs (id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		entry.ID, entry.Time, entry.Method, entry.Path, entry.UpstreamURL, entry.CLIType, entry.ProviderID, entry.Provider, entry.Model, entry.StatusCode, entry.Duration, entry.PromptTokens, entry.CompletionTokens, entry.TotalTokens, entry.Error, entry.ResponseBody,
	)
	if err != nil {
		slog.Error("failed to write request log", "error", err)
	}

	// 异步写入用量，不阻塞日志主流程
	go l.addProviderUsage(entry)
}

func (l *Logger) GetLogs() []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query(
		"SELECT id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs ORDER BY time DESC LIMIT ?",
		maxLogCount,
	)
	if err != nil {
		return []RequestLog{}
	}
	defer rows.Close()

	logs := make([]RequestLog, 0)
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.UpstreamURL, &log.CLIType, &log.ProviderID, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
}

// GetLogs retrieves logs up to a specified limit, newest first.
func (l *Logger) GetLogsWithLimit(limit int) []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query(
		"SELECT id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs ORDER BY time DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return []RequestLog{}
	}
	defer rows.Close()

	logs := make([]RequestLog, 0)
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.UpstreamURL, &log.CLIType, &log.ProviderID, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
}

// GetLogsSince retrieves logs with ID greater than lastID.
// If lastID is empty, returns all logs (up to maxLogCount).
func (l *Logger) GetLogsSince(lastID string) []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var rows *sql.Rows
	var err error

	if lastID == "" {
		rows, err = l.db.Query(
			"SELECT id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs ORDER BY time DESC LIMIT ?",
			maxLogCount,
		)
	} else {
		rows, err = l.db.Query(
			"SELECT id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs WHERE id > ? ORDER BY time DESC LIMIT ?",
			lastID, maxLogCount,
		)
	}

	if err != nil {
		return []RequestLog{}
	}
	defer rows.Close()

	logs := make([]RequestLog, 0)
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.UpstreamURL, &log.CLIType, &log.ProviderID, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
}

// GetLogsByTimeRange retrieves logs within [from, to] time range (UnixMillis).
// Returns up to maxLogCount entries, newest first.
func (l *Logger) GetLogsByTimeRange(from, to int64) []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query(
		"SELECT id, time, method, path, upstream_url, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs WHERE time >= ? AND time <= ? ORDER BY time DESC LIMIT ?",
		from, to, maxLogCount,
	)
	if err != nil {
		return []RequestLog{}
	}
	defer rows.Close()

	logs := make([]RequestLog, 0)
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.UpstreamURL, &log.CLIType, &log.ProviderID, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
}

func (l *Logger) GetTotalTokens() (in, out, total int64) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.db.QueryRow("SELECT COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0) FROM providers").Scan(&in, &out, &total)
	return
}

func (l *Logger) GetProviderUsageStats() []ProviderUsageStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query(`
		SELECT
			id,
			name,
			COALESCE(prompt_tokens, 0),
			COALESCE(completion_tokens, 0),
			COALESCE(total_tokens, 0),
			COALESCE(usage_updated_at, 0)
		FROM providers
		ORDER BY created_at
	`)
	if err != nil {
		return []ProviderUsageStats{}
	}
	defer rows.Close()

	stats := make([]ProviderUsageStats, 0)
	for rows.Next() {
		var stat ProviderUsageStats
		err := rows.Scan(&stat.ProviderID, &stat.Provider, &stat.PromptTokens, &stat.CompletionTokens, &stat.TotalTokens, &stat.UpdatedAt)
		if err != nil {
			continue
		}
		stats = append(stats, stat)
	}
	return stats
}

func (l *Logger) GetProviderUsageSeries(providerID string) []ProviderUsagePoint {
	l.mu.RLock()
	defer l.mu.RUnlock()

	cutoff := time.Now().Add(-usageRetention).UnixMilli()
	rows, err := l.db.Query(`
		SELECT bucket_start, prompt_tokens, completion_tokens, total_tokens
		FROM provider_usage_points
		WHERE provider_id = ?
		AND bucket_start > ?
		ORDER BY bucket_start ASC
	`, providerID, cutoff)
	if err != nil {
		return []ProviderUsagePoint{}
	}
	defer rows.Close()

	points := make([]ProviderUsagePoint, 0)
	for rows.Next() {
		var point ProviderUsagePoint
		err := rows.Scan(&point.Time, &point.PromptTokens, &point.CompletionTokens, &point.TotalTokens)
		if err != nil {
			continue
		}
		points = append(points, point)
	}
	return points
}

func (l *Logger) addProviderUsage(entry RequestLog) {
	if entry.ProviderID == "" {
		return
	}
	if entry.PromptTokens == 0 && entry.CompletionTokens == 0 && entry.TotalTokens == 0 {
		return
	}

	_, err := l.db.Exec(
		`UPDATE providers
		SET prompt_tokens = COALESCE(prompt_tokens, 0) + ?,
			completion_tokens = COALESCE(completion_tokens, 0) + ?,
			total_tokens = COALESCE(total_tokens, 0) + ?,
			usage_updated_at = ?
		WHERE id = ?`,
		entry.PromptTokens,
		entry.CompletionTokens,
		entry.TotalTokens,
		entry.Time,
		entry.ProviderID,
	)
	if err != nil {
		slog.Error("failed to update provider usage", "error", err)
	}

	bucketStart := entry.Time - entry.Time%int64(10*time.Minute/time.Millisecond)
	_, err = l.db.Exec(
		`INSERT INTO provider_usage_points (provider_id, bucket_start, prompt_tokens, completion_tokens, total_tokens)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider_id, bucket_start) DO UPDATE SET
			prompt_tokens = prompt_tokens + excluded.prompt_tokens,
			completion_tokens = completion_tokens + excluded.completion_tokens,
			total_tokens = total_tokens + excluded.total_tokens`,
		entry.ProviderID,
		bucketStart,
		entry.PromptTokens,
		entry.CompletionTokens,
		entry.TotalTokens,
	)
	if err != nil {
		slog.Error("failed to update provider usage series", "error", err)
	}
}

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.db.Exec("DELETE FROM request_logs")
}

func (l *Logger) GetSizeKB() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var count int64
	l.db.QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&count)
	return (count * 500) / 1024
}
