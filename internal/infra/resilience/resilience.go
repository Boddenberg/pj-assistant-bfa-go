// Package resilience provides fault-tolerance patterns:
// retry with exponential backoff, circuit breaker, and bulkhead.
package resilience

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/sony/gobreaker"
)

// Config holds resilience parameters.
type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxConcurrency int
}

// RetryWithBackoff executes fn with exponential backoff + jitter.
// It respects context cancellation.
func RetryWithBackoff(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if attempt < cfg.MaxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * cfg.InitialBackoff
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			wait := backoff + jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return lastErr
}

// NewCircuitBreaker creates a circuit breaker with sensible defaults.
func NewCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,                // half-open: allow 3 requests
		Interval:    30 * time.Second, // closed: reset counters every 30s
		Timeout:     10 * time.Second, // open -> half-open after 10s
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
	})
}

// Bulkhead limits concurrent access to a resource.
type Bulkhead struct {
	sem chan struct{}
}

// NewBulkhead creates a bulkhead with the given max concurrency.
func NewBulkhead(maxConcurrency int) *Bulkhead {
	return &Bulkhead{sem: make(chan struct{}, maxConcurrency)}
}

// Acquire blocks until a slot is available or context is cancelled.
func (b *Bulkhead) Acquire(ctx context.Context) error {
	select {
	case b.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees a slot.
func (b *Bulkhead) Release() {
	<-b.sem
}
