package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/config"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/handler"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/cache"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/client"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"go.uber.org/zap"
)

func main() {
	// --- Load .env file (for local development) ---
	_ = config.LoadDotEnv(".env")

	// --- Config ---
	cfg := config.Load()

	// --- Logger ---
	logger := observability.NewLogger(cfg.LogLevel)
	defer logger.Sync()

	logger.Info("configuration loaded",
		zap.Int("port", cfg.Port),
		zap.String("log_level", cfg.LogLevel),
		zap.Bool("use_supabase", cfg.UseSupabase),
		zap.Duration("http_timeout", cfg.HTTPTimeout),
		zap.Duration("cache_ttl", cfg.CacheTTL),
		zap.Int("max_retries", cfg.MaxRetries),
		zap.Duration("initial_backoff", cfg.InitialBackoff),
		zap.Duration("jwt_access_ttl", cfg.JWTAccessTTL),
		zap.Duration("jwt_refresh_ttl", cfg.JWTRefreshTTL),
	)

	// --- Tracing ---
	shutdown, err := observability.InitTracer(cfg.OTLPEndpoint, "pj-assistant-bfa")
	if err != nil {
		logger.Fatal("failed to init tracer", zap.Error(err))
	}
	defer shutdown(context.Background())

	// --- Metrics ---
	metrics := observability.NewMetrics()

	// --- Cache ---
	profileCache := cache.New[any](cfg.CacheTTL)

	// --- Resilience ---
	resilienceCfg := resilience.Config{
		MaxRetries:     cfg.MaxRetries,
		InitialBackoff: cfg.InitialBackoff,
		MaxConcurrency: cfg.MaxConcurrency,
	}
	cb := resilience.NewCircuitBreaker("external-apis")

	// --- Clients ---
	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}

	var profileClient port.ProfileFetcher
	var transactionsClient port.TransactionsFetcher
	var supabaseClient *supabase.Client

	if cfg.UseSupabase && cfg.SupabaseURL != "" {
		logger.Info("using Supabase as data backend",
			zap.String("supabase_url", cfg.SupabaseURL),
		)
		supabaseClient = supabase.NewClient(
			httpClient,
			cfg.SupabaseURL,
			cfg.SupabaseAnonKey,
			cfg.SupabaseServiceKey,
			cb,
			resilienceCfg,
			logger,
		)
		profileClient = supabaseClient
		transactionsClient = supabaseClient
	} else {
		logger.Info("using HTTP API clients as data backend")
		profileClient = client.NewProfileClient(httpClient, cfg.ProfileAPIURL, cb, resilienceCfg)
		transactionsClient = client.NewTransactionsClient(httpClient, cfg.TransactionsAPIURL, cb, resilienceCfg)
	}

	agentClient := client.NewAgentClient(httpClient, cfg.AgentAPIURL, cb, resilienceCfg)

	// --- Services ---
	assistantSvc := service.NewAssistant(
		profileClient,
		transactionsClient,
		agentClient,
		profileCache,
		metrics,
		logger,
	)

	// Banking service (uses Supabase as store)
	var bankSvc *service.BankingService
	var authSvc *service.AuthService
	if supabaseClient != nil {
		bankSvc = service.NewBankingService(supabaseClient, metrics, logger)
		logger.Info("banking service enabled with Supabase store")

		authSvc = service.NewAuthService(supabaseClient, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, logger)
		logger.Info("auth service enabled")
	} else {
		logger.Warn("banking service: Supabase not configured, banking routes unavailable")
		logger.Warn("auth service: Supabase not configured, auth routes unavailable")
	}

	// --- Router ---
	router := handler.NewRouter(assistantSvc, bankSvc, authSvc, metrics, logger)

	// --- Server ---
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Graceful shutdown ---
	go func() {
		logger.Info("server starting", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("server shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("server forced shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
