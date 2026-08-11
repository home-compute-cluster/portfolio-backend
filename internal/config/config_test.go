package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("DB_PASSWORD", "configured-password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Address != ":8080" {
		t.Fatalf("HTTP address = %q, want %q", cfg.HTTP.Address, ":8080")
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("read header timeout = %s, want %s", cfg.HTTP.ReadHeaderTimeout, 5*time.Second)
	}
	if cfg.Database.MaxConns != 5 || cfg.Database.MinConns != 0 {
		t.Fatalf("pool sizes = (%d, %d), want (5, 0)", cfg.Database.MaxConns, cfg.Database.MinConns)
	}
	if cfg.Comments.MaxAuthorChars != 80 || cfg.Comments.MaxCommentChars != 2000 || cfg.Comments.MaxCommentsPerPost != 1000 {
		t.Fatalf("comment defaults = %#v", cfg.Comments)
	}
	if cfg.Views.DeduplicationWindow != 24*time.Hour {
		t.Fatalf("view window = %s, want 24h", cfg.Views.DeduplicationWindow)
	}
	if cfg.Database.Password != "configured-password" {
		t.Fatal("database password was not loaded from DB_PASSWORD")
	}
}

func TestLoadAPIRequiresStrongVisitorKey(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")

	if _, err := LoadAPI(); err == nil || !strings.Contains(err.Error(), "VISITOR_HMAC_KEY") {
		t.Fatalf("LoadAPI() error = %v, want visitor key error", err)
	}

	t.Setenv("VISITOR_HMAC_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("CF_ACCESS_TEAM_DOMAIN", "https://example.cloudflareaccess.com")
	t.Setenv("CF_ACCESS_AUD", "test-audience")
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	if _, err := LoadAPI(); err != nil {
		t.Fatalf("LoadAPI() with strong key error = %v", err)
	}
}

func TestLoadAPIRequiresCloudflareAccessConfiguration(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("VISITOR_HMAC_KEY", "0123456789abcdef0123456789abcdef")

	for _, test := range []struct {
		name  string
		value string
	}{
		{"CF_ACCESS_TEAM_DOMAIN", "https://example.cloudflareaccess.com"},
		{"CF_ACCESS_AUD", "test-audience"},
		{"ADMIN_EMAIL", "admin@example.com"},
	} {
		if _, err := LoadAPI(); err == nil || !strings.Contains(err.Error(), test.name) {
			t.Fatalf("without %s LoadAPI() error = %v", test.name, err)
		}
		t.Setenv(test.name, test.value)
	}
	if _, err := LoadAPI(); err != nil {
		t.Fatalf("complete Access configuration error = %v", err)
	}
}

func TestLoadValidatesAndNormalizesAccessTeamDomain(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("CF_ACCESS_TEAM_DOMAIN", "https://example.cloudflareaccess.com/")
	t.Setenv("ADMIN_EMAIL", " Admin@Example.COM ")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Access.TeamDomain != "https://example.cloudflareaccess.com" || cfg.Access.AdminEmail != "admin@example.com" {
		t.Fatalf("Access configuration = %#v", cfg.Access)
	}

	t.Setenv("CF_ACCESS_TEAM_DOMAIN", "http://example.cloudflareaccess.com/path")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CF_ACCESS_TEAM_DOMAIN") {
		t.Fatalf("invalid team domain error = %v", err)
	}
}

func TestLoadParsesTrustedProxyCIDRs(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 2001:db8::/32")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Security.TrustedProxyCIDRs) != 2 {
		t.Fatalf("trusted proxies = %v, want two prefixes", cfg.Security.TrustedProxyCIDRs)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("invalid proxy CIDR error = %v", err)
	}
}

func TestLoadRejectsLimitsAboveDatabaseConstraints(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")

	t.Setenv("MAX_AUTHOR_CHARS", "81")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MAX_AUTHOR_CHARS") {
		t.Fatalf("author limit error = %v", err)
	}
	t.Setenv("MAX_AUTHOR_CHARS", "80")
	t.Setenv("MAX_COMMENT_CHARS", "2001")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MAX_COMMENT_CHARS") {
		t.Fatalf("comment limit error = %v", err)
	}
	t.Setenv("MAX_COMMENT_CHARS", "2000")
	t.Setenv("VIEW_DEDUP_WINDOW_HOURS", "8761")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "VIEW_DEDUP_WINDOW_HOURS") {
		t.Fatalf("view window error = %v", err)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	clearEnvironment(t)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v, want missing DATABASE_URL error", err)
	}
}

func TestLoadRejectsMalformedDuration(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("HTTP_READ_TIMEOUT", "eventually")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "HTTP_READ_TIMEOUT") {
		t.Fatalf("Load() error = %v, want HTTP_READ_TIMEOUT error", err)
	}
}

func TestLoadRejectsInvalidPoolBounds(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("DB_MIN_CONNS", "6")
	t.Setenv("DB_MAX_CONNS", "5")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "pool sizes") {
		t.Fatalf("Load() error = %v, want pool size error", err)
	}
}

func clearEnvironment(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"DATABASE_URL",
		"DB_PASSWORD",
		"DB_MAX_CONNS",
		"DB_MIN_CONNS",
		"HTTP_ADDR",
		"HTTP_READ_HEADER_TIMEOUT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"HTTP_SHUTDOWN_TIMEOUT",
		"DB_CONNECT_TIMEOUT",
		"DB_READINESS_TIMEOUT",
		"DB_QUERY_TIMEOUT",
		"MAX_AUTHOR_CHARS",
		"MAX_COMMENT_CHARS",
		"MAX_COMMENTS_PER_POST",
		"VISITOR_HMAC_KEY",
		"TRUSTED_PROXY_CIDRS",
		"VIEW_DEDUP_WINDOW_HOURS",
		"CF_ACCESS_TEAM_DOMAIN",
		"CF_ACCESS_AUD",
		"ADMIN_EMAIL",
	} {
		t.Setenv(name, "")
	}
}
