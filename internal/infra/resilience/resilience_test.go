package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"
)

func TestRetryWithBackoff_Success(t *testing.T) {
	cfg := resilience.Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}

	callCount := 0
	err := resilience.RetryWithBackoff(context.Background(), cfg, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRetryWithBackoff_RetriesOnFailure(t *testing.T) {
	cfg := resilience.Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}

	callCount := 0
	err := resilience.RetryWithBackoff(context.Background(), cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetryWithBackoff_ExhaustsRetries(t *testing.T) {
	cfg := resilience.Config{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
	}

	err := resilience.RetryWithBackoff(context.Background(), cfg, func() error {
		return errors.New("persistent error")
	})

	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestRetryWithBackoff_RespectsContext(t *testing.T) {
	cfg := resilience.Config{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := resilience.RetryWithBackoff(ctx, cfg, func() error {
		return errors.New("error")
	})

	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestBulkhead_AcquireRelease(t *testing.T) {
	bh := resilience.NewBulkhead(2)

	if err := bh.Acquire(context.Background()); err != nil {
		t.Fatalf("expected acquire, got %v", err)
	}
	if err := bh.Acquire(context.Background()); err != nil {
		t.Fatalf("expected acquire, got %v", err)
	}

	// Third acquire should block — test with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := bh.Acquire(ctx)
	if err == nil {
		t.Fatal("expected timeout on third acquire")
	}

	// Release one slot
	bh.Release()

	if err := bh.Acquire(context.Background()); err != nil {
		t.Fatalf("expected acquire after release, got %v", err)
	}
}
