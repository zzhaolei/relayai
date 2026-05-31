package proxy

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestGetProviderUsageSeries(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
		CREATE TABLE providers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			default_model TEXT,
			model_mappings TEXT,
			cli_types TEXT,
			enabled INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			usage_updated_at INTEGER DEFAULT 0
		);
		CREATE TABLE request_logs (
			id TEXT PRIMARY KEY,
			time INTEGER NOT NULL,
			method TEXT,
			path TEXT,
			upstream_url TEXT,
			cli_type TEXT,
			provider_id TEXT,
			provider TEXT,
			model TEXT,
			status_code INTEGER,
			duration_ms INTEGER,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			error TEXT,
			response_body TEXT
		);
		CREATE TABLE provider_usage_points (
			provider_id TEXT NOT NULL,
			bucket_start INTEGER NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			PRIMARY KEY (provider_id, bucket_start)
		)
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO providers (id, name, base_url, api_key, created_at) VALUES
			('provider-openai', 'OpenAI', 'https://example.test/openai', 'key', 1),
			('provider-anthropic', 'Anthropic', 'https://example.test/anthropic', 'key', 2)
	`)
	if err != nil {
		t.Fatalf("insert providers: %v", err)
	}

	logger := NewLogger(db)
	logger.Add(RequestLog{ProviderID: "provider-openai", Provider: "OpenAI", PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
	logger.Add(RequestLog{ProviderID: "provider-openai", Provider: "OpenAI", PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10})
	logger.Add(RequestLog{ProviderID: "provider-anthropic", Provider: "Anthropic", PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10})
	logger.Add(RequestLog{Provider: "OpenAI", PromptTokens: 100, CompletionTokens: 100, TotalTokens: 200})

	time.Sleep(100 * time.Millisecond)

	series := logger.GetProviderUsageSeries("provider-openai")
	if len(series) != 1 {
		t.Fatalf("expected 1 OpenAI usage point, got %d", len(series))
	}
	if series[0].PromptTokens != 17 || series[0].CompletionTokens != 8 || series[0].TotalTokens != 25 {
		t.Fatalf("unexpected OpenAI usage series: %+v", series[0])
	}
}
