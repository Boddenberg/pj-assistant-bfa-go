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
func NewLogger(level string) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if level == "debug" {
		cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		panic("failed to create logger: " + err.Error())
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
