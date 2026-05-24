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
	ID               string `json:"id"`
	Time             int64  `json:"time"`
	Method           string `json:"method"`
	Path             string `json:"path"`
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
		"INSERT INTO request_logs (id, time, method, path, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		entry.ID, entry.Time, entry.Method, entry.Path, entry.CLIType, entry.ProviderID, entry.Provider, entry.Model, entry.StatusCode, entry.Duration, entry.PromptTokens, entry.CompletionTokens, entry.TotalTokens, entry.Error, entry.ResponseBody,
	)
	if err != nil {
		// 日志写入失败不影响主流程
		fmt.Printf("写入日志失败: %v\n", err)
	}

	l.addProviderUsage(entry)
}

func (l *Logger) GetLogs() []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rows, err := l.db.Query("SELECT id, time, method, path, cli_type, provider_id, provider, model, status_code, duration_ms, prompt_tokens, completion_tokens, total_tokens, error, response_body FROM request_logs ORDER BY time DESC LIMIT 500")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.Time, &log.Method, &log.Path, &log.CLIType, &log.ProviderID, &log.Provider, &log.Model, &log.StatusCode, &log.Duration, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Error, &log.ResponseBody)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	return logs
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
		return nil
	}
	defer rows.Close()

	var stats []ProviderUsageStats
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

	rows, err := l.db.Query(`
		SELECT bucket_start, prompt_tokens, completion_tokens, total_tokens
		FROM provider_usage_points
		WHERE provider_id = ?
		ORDER BY bucket_start DESC
		LIMIT 120
	`, providerID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var reversed []ProviderUsagePoint
	for rows.Next() {
		var point ProviderUsagePoint
		err := rows.Scan(&point.Time, &point.PromptTokens, &point.CompletionTokens, &point.TotalTokens)
		if err != nil {
			continue
		}
		reversed = append(reversed, point)
	}

	points := make([]ProviderUsagePoint, len(reversed))
	for i := range reversed {
		points[len(reversed)-1-i] = reversed[i]
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
		// 用量累计失败不影响代理请求。
		fmt.Printf("更新提供商用量失败: %v\n", err)
	}

	bucketStart := entry.Time - entry.Time%int64(time.Minute/time.Millisecond)
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
		// 曲线采样失败不影响代理请求。
		fmt.Printf("更新提供商用量曲线失败: %v\n", err)
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
	// 估算每条日志约 500 字节
	return (count * 500) / 1024
}
