package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
// Values are loaded from environment variables with sensible defaults.
type Config struct {
	// Server
	Port     int
	LogLevel string

	// External services
	ProfileAPIURL      string
	TransactionsAPIURL string
	AgentAPIURL        string
	ChatAgentURL       string // URL do Agent Python para o chat (POST /v1/chat)

	// HTTP client
	HTTPTimeout time.Duration

	// Resilience
	MaxRetries     int
	InitialBackoff time.Duration
	MaxConcurrency int

	// Cache
	CacheTTL time.Duration

	// Observability
	OTLPEndpoint string

	// Supabase
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string
	UseSupabase        bool

	// JWT / Auth
	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	// Dev mode
	DevAuth bool // DEV_AUTH=true bypasses bcrypt, uses dev_logins table
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		Port:     getEnvInt("PORT", 8080),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		ProfileAPIURL:      getEnv("PROFILE_API_URL", "http://localhost:8081"),
		TransactionsAPIURL: getEnv("TRANSACTIONS_API_URL", "http://localhost:8082"),
		AgentAPIURL:        getEnv("AGENT_API_URL", "http://localhost:8090"),
		ChatAgentURL:       getEnv("CHAT_AGENT_URL", "https://pj-assistant-agent-py-production.up.railway.app"),

		HTTPTimeout: getEnvDuration("HTTP_TIMEOUT", 10*time.Second),

		MaxRetries:     getEnvInt("MAX_RETRIES", 3),
		InitialBackoff: getEnvDuration("INITIAL_BACKOFF", 100*time.Millisecond),
		MaxConcurrency: getEnvInt("MAX_CONCURRENCY", 50),

		CacheTTL: getEnvDuration("CACHE_TTL", 5*time.Minute),

		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),

		SupabaseURL:        getEnv("SUPABASE_URL", ""),
		SupabaseAnonKey:    getEnv("SUPABASE_ANON_KEY", ""),
		SupabaseServiceKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),
		UseSupabase:        getEnv("USE_SUPABASE", "true") == "true",

		JWTSecret:     getEnv("JWT_SECRET", "bfa-default-dev-secret-change-me"),
		JWTAccessTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),

		DevAuth: getEnv("DEV_AUTH", "false") == "true",
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
