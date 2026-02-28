package observability

import (
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// Metrics holds all Prometheus metrics for the BFA.
type Metrics struct {
	// Registry is the Prometheus registry that owns these metrics.
	// Exposed so the /metrics endpoint can use it.
	Registry *prometheus.Registry

	requestDuration *prometheus.HistogramVec
	externalErrors  *prometheus.CounterVec
	cacheHits       *prometheus.CounterVec
	cacheMisses     *prometheus.CounterVec
	tokensUsed      *prometheus.CounterVec
	requestsTotal   *prometheus.CounterVec
}

// NewMetrics creates a dedicated Prometheus registry and registers all
// application metrics in it. Using a private registry avoids "duplicate
// collector" panics when NewMetrics is called more than once (e.g. in tests).
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Metrics{
		Registry: reg,

		requestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "bfa_request_duration_seconds",
				Help:    "Duration of requests by operation.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		externalErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bfa_external_errors_total",
				Help: "Total errors from external services.",
			},
			[]string{"service"},
		),
		cacheHits: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bfa_cache_hits_total",
				Help: "Total cache hits.",
			},
			[]string{"cache"},
		),
		cacheMisses: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bfa_cache_misses_total",
				Help: "Total cache misses.",
			},
			[]string{"cache"},
		),
		tokensUsed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bfa_llm_tokens_total",
				Help: "Total LLM tokens consumed.",
			},
			[]string{"type"},
		),
		requestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bfa_requests_total",
				Help: "Total requests processed.",
			},
			[]string{"status"},
		),
	}
}

// RecordRequestDuration records the duration of an operation.
func (m *Metrics) RecordRequestDuration(operation string, d time.Duration) {
	m.requestDuration.WithLabelValues(operation).Observe(d.Seconds())
}

// IncrExternalError increments the external error counter.
func (m *Metrics) IncrExternalError(service string) {
	m.externalErrors.WithLabelValues(service).Inc()
}

// IncrCacheHit increments the cache hit counter.
func (m *Metrics) IncrCacheHit(cache string) {
	m.cacheHits.WithLabelValues(cache).Inc()
}

// IncrCacheMiss increments the cache miss counter.
func (m *Metrics) IncrCacheMiss(cache string) {
	m.cacheMisses.WithLabelValues(cache).Inc()
}

// RecordTokens records prompt and completion token usage.
func (m *Metrics) RecordTokens(prompt, completion int) {
	m.tokensUsed.WithLabelValues("prompt").Add(float64(prompt))
	m.tokensUsed.WithLabelValues("completion").Add(float64(completion))
}

// IncrRequest increments the request counter with a status label.
func (m *Metrics) IncrRequest(status string) {
	m.requestsTotal.WithLabelValues(status).Inc()
}

// GetAgentSnapshot returns a snapshot of agent-related metrics suitable for the
// GET /v1/metrics/agent endpoint.
func (m *Metrics) GetAgentSnapshot() *domain.AgentMetrics {
	// Gather current values from Prometheus counters.
	// Note: Prometheus counters expose cumulative values.
	promptTokens := getCounterValue(m.tokensUsed, "prompt")
	completionTokens := getCounterValue(m.tokensUsed, "completion")
	totalRequests := getCounterValue(m.requestsTotal, "success") +
		getCounterValue(m.requestsTotal, "error")
	errorCount := getCounterValue(m.requestsTotal, "error")
	cacheHits := getCounterValue(m.cacheHits, "profile")
	cacheMisses := getCounterValue(m.cacheMisses, "profile")

	totalTokens := promptTokens + completionTokens
	avgTokens := float64(0)
	errorRate := float64(0)
	cacheHitRate := float64(0)

	if totalRequests > 0 {
		avgTokens = totalTokens / totalRequests
		errorRate = errorCount / totalRequests
	}
	if cacheHits+cacheMisses > 0 {
		cacheHitRate = cacheHits / (cacheHits + cacheMisses)
	}

	// Estimated cost: ~$0.03/1k prompt tokens, ~$0.06/1k completion tokens (GPT-4o)
	estimatedCost := (promptTokens/1000)*0.03 + (completionTokens/1000)*0.06

	return &domain.AgentMetrics{
		TotalRequests:       int64(totalRequests),
		AvgLatencyMs:        0, // Would need histogram observation; stub for now
		P95LatencyMs:        0,
		P99LatencyMs:        0,
		ErrorRate:           errorRate,
		FallbackRate:        0,
		AvgTokensPerRequest: avgTokens,
		EstimatedCostUsd:    estimatedCost,
		RAGPrecision:        0,
		CacheHitRate:        cacheHitRate,
		Period:              "all_time",
	}
}

// getCounterValue extracts the current float64 value from a CounterVec for a given label.
func getCounterValue(cv *prometheus.CounterVec, label string) float64 {
	counter := cv.WithLabelValues(label)
	m := &dto.Metric{}
	if err := counter.(prometheus.Metric).Write(m); err != nil {
		return 0
	}
	if m.Counter != nil && m.Counter.Value != nil {
		return *m.Counter.Value
	}
	return 0
}
