package observability

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a structured zap logger.
// Always uses production base (no stacktraces on Warn).
// debug level → colorized console; otherwise → compact JSON.
// If axiomToken is provided, logs are also sent to Axiom.
func NewLogger(level string, axiomToken, axiomDataset string) *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// Logs normais vão para stdout (evita vermelho no GoLand/IDEs que colorem stderr)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	var zapLevel zapcore.Level
	if level == "debug" {
		zapLevel = zapcore.DebugLevel
		cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	} else {
		zapLevel = zapcore.InfoLevel
		cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	logger, err := cfg.Build()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}

	// Se Axiom configurado, adiciona core extra (tee)
	if axiomToken != "" && axiomDataset != "" {
		axCore := newAxiomCore(axiomConfig{
			Token:   axiomToken,
			Dataset: axiomDataset,
			Level:   zapLevel,
		})
		if axCore != nil {
			teeCore := zapcore.NewTee(logger.Core(), axCore)
			logger = zap.New(teeCore, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
			logger.Info("axiom logging enabled", zap.String("dataset", axiomDataset))
		}
	} else {
		logger.Warn("axiom logging DISABLED — set AXIOM_TOKEN and AXIOM_DATASET to enable")
	}

	return logger
}

// ZapLoggerMiddleware logs HTTP requests with zap.
// Uses Warn for 4xx, Error for 5xx, Info for 2xx/3xx.
func ZapLoggerMiddleware(logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				fields := []zap.Field{
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", status),
					zap.Duration("latency", time.Since(start)),
					zap.String("request_id", middleware.GetReqID(r.Context())),
					zap.String("remote_addr", r.RemoteAddr),
				}

				switch {
				case status >= 500:
					logger.Error("http request", fields...)
				case status >= 400:
					logger.Warn("http request", fields...)
				default:
					logger.Info("http request", fields...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// TracingMiddleware extracts trace context from incoming requests.
func TracingMiddleware(next http.Handler) http.Handler {
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
