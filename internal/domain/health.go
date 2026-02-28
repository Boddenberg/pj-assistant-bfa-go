package domain

// ============================================================
// Health & Metrics API Responses
// ============================================================

// HealthStatus is returned by GET /healthz.
type HealthStatus struct {
	Status   string          `json:"status"` // healthy, degraded, unhealthy
	Services []ServiceHealth `json:"services"`
}

// ServiceHealth represents the health of an individual service.
type ServiceHealth struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	LatencyMs     int64   `json:"latencyMs"`
	UptimePercent float64 `json:"uptimePercent"`
	LastChecked   string  `json:"lastChecked"`
}

// AgentMetrics is returned by GET /v1/metrics/agent.
type AgentMetrics struct {
	TotalRequests       int64   `json:"totalRequests"`
	AvgLatencyMs        float64 `json:"avgLatencyMs"`
	P95LatencyMs        float64 `json:"p95LatencyMs"`
	P99LatencyMs        float64 `json:"p99LatencyMs"`
	ErrorRate           float64 `json:"errorRate"`
	FallbackRate        float64 `json:"fallbackRate"`
	AvgTokensPerRequest float64 `json:"avgTokensPerRequest"`
	EstimatedCostUsd    float64 `json:"estimatedCostUsd"`
	RAGPrecision        float64 `json:"ragPrecision"`
	CacheHitRate        float64 `json:"cacheHitRate"`
	Period              string  `json:"period"`
}

// ============================================================
// Generic API Response wrappers
// ============================================================

// ListResponse wraps paginated list results.
type ListResponse[T any] struct {
	Data     []T  `json:"data"`
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasMore  bool `json:"has_more"`
}

// SuccessResponse wraps a successful single-entity response.
type SuccessResponse struct {
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}
