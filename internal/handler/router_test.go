package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/handler"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"

	"go.uber.org/zap"
)

func TestHealthz(t *testing.T) {
	router := handler.NewRouter(nil, nil, nil, observability.NewMetrics(), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	router := handler.NewRouter(nil, nil, nil, observability.NewMetrics(), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMetrics(t *testing.T) {
	router := handler.NewRouter(nil, nil, nil, observability.NewMetrics(), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
