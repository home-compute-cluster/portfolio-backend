package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTP     HTTPConfig
	Database DatabaseConfig
	Comments CommentsConfig
	Security SecurityConfig
	Views    ViewsConfig
}

type HTTPConfig struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type DatabaseConfig struct {
	URL              string
	Password         string
	MaxConns         int32
	MinConns         int32
	ConnectTimeout   time.Duration
	ReadinessTimeout time.Duration
	QueryTimeout     time.Duration
}

type CommentsConfig struct {
	MaxAuthorChars     int
	MaxCommentChars    int
	MaxCommentsPerPost int
	DefaultPageSize    int
	MaximumPageSize    int
}

type SecurityConfig struct {
	VisitorHMACKey    []byte
	TrustedProxyCIDRs []netip.Prefix
}

type ViewsConfig struct {
	DeduplicationWindow time.Duration
}

func LoadAPI() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return Config{}, err
	}
	if len(cfg.Security.VisitorHMACKey) < 32 {
		return Config{}, errors.New("VISITOR_HMAC_KEY must contain at least 32 bytes")
	}
	return cfg, nil
}

func Load() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL environment variable is required")
	}

	maxConns, err := envInt32("DB_MAX_CONNS", 5)
	if err != nil {
		return Config{}, err
	}

	minConns, err := envInt32("DB_MIN_CONNS", 0)
	if err != nil {
		return Config{}, err
	}

	if minConns < 0 || maxConns < 1 || minConns > maxConns {
		return Config{}, errors.New("database pool sizes must satisfy 0 <= DB_MIN_CONNS <= DB_MAX_CONNS")
	}

	readHeaderTimeout, err := envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := envDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := envDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := envDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	connectTimeout, err := envDuration("DB_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	readinessTimeout, err := envDuration("DB_READINESS_TIMEOUT", 750*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	queryTimeout, err := envDuration("DB_QUERY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}

	maxAuthorChars, err := envPositiveInt("MAX_AUTHOR_CHARS", 80)
	if err != nil {
		return Config{}, err
	}
	maxCommentChars, err := envPositiveInt("MAX_COMMENT_CHARS", 2000)
	if err != nil {
		return Config{}, err
	}
	maxCommentsPerPost, err := envPositiveInt("MAX_COMMENTS_PER_POST", 1000)
	if err != nil {
		return Config{}, err
	}

	trustedProxyCIDRs, err := envPrefixes("TRUSTED_PROXY_CIDRS")
	if err != nil {
		return Config{}, err
	}
	viewWindowHours, err := envPositiveInt("VIEW_DEDUP_WINDOW_HOURS", 24)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP: HTTPConfig{
			Address:           envString("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ShutdownTimeout:   shutdownTimeout,
		},
		Database: DatabaseConfig{
			URL:              databaseURL,
			Password:         os.Getenv("DB_PASSWORD"),
			MaxConns:         maxConns,
			MinConns:         minConns,
			ConnectTimeout:   connectTimeout,
			ReadinessTimeout: readinessTimeout,
			QueryTimeout:     queryTimeout,
		},
		Comments: CommentsConfig{
			MaxAuthorChars:     maxAuthorChars,
			MaxCommentChars:    maxCommentChars,
			MaxCommentsPerPost: maxCommentsPerPost,
			DefaultPageSize:    25,
			MaximumPageSize:    100,
		},
		Security: SecurityConfig{
			VisitorHMACKey:    []byte(os.Getenv("VISITOR_HMAC_KEY")),
			TrustedProxyCIDRs: trustedProxyCIDRs,
		},
		Views: ViewsConfig{
			DeduplicationWindow: time.Duration(viewWindowHours) * time.Hour,
		},
	}, nil
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func envInt32(name string, fallback int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a 32-bit integer: %w", name, err)
	}

	return int32(parsed), nil
}

func envPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func envPrefixes(name string) ([]netip.Prefix, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}

	parts := strings.Split(value, ",")
	result := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("%s contains an invalid CIDR", name)
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration: %w", name, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}

	return parsed, nil
}
