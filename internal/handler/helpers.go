package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
)

// ============================================================
// Shared helper functions
// ============================================================

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}
	return
}

// formatKeyValue returns a human-readable formatted version of a pix key value.
func formatKeyValue(keyType, value string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	switch keyType {
	case "cnpj":
		if len(digits) == 14 {
			return fmt.Sprintf("%s.%s.%s/%s-%s", digits[:2], digits[2:5], digits[5:8], digits[8:12], digits[12:14])
		}
	case "cpf":
		if len(digits) == 11 {
			return fmt.Sprintf("%s.%s.%s-%s", digits[:3], digits[3:6], digits[6:9], digits[9:11])
		}
	case "phone":
		if len(digits) == 11 {
			return fmt.Sprintf("(%s) %s-%s", digits[:2], digits[2:7], digits[7:11])
		} else if len(digits) == 13 { // +55...
			return fmt.Sprintf("+%s (%s) %s-%s", digits[:2], digits[2:4], digits[4:9], digits[9:13])
		}
	}
	return value
}

// handleServiceError maps domain errors to HTTP responses.
func handleServiceError(w http.ResponseWriter, err error, logger *zap.Logger) {
	var notFound *domain.ErrNotFound
	var circuitOpen *domain.ErrCircuitOpen
	var timeout *domain.ErrTimeout
	var validation *domain.ErrValidation
	var insufficientFunds *domain.ErrInsufficientFunds
	var limitExceeded *domain.ErrLimitExceeded
	var duplicate *domain.ErrDuplicate
	var forbidden *domain.ErrForbidden
	var invalidBarcode *domain.ErrInvalidBarcode
	var unauthorized *domain.ErrUnauthorized
	var accountBlocked *domain.ErrAccountBlocked
	var conflict *domain.ErrConflict
	var invalidCode *domain.ErrInvalidCode

	switch {
	case errors.As(err, &notFound):
		logger.Debug("not found", zap.String("error", err.Error()))
		writeError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &circuitOpen):
		logger.Error("circuit breaker open", zap.Error(err))
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &timeout):
		logger.Error("request timeout", zap.Error(err))
		writeError(w, http.StatusGatewayTimeout, err.Error())
	case errors.As(err, &validation):
		logger.Debug("validation error", zap.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &insufficientFunds):
		logger.Warn("insufficient funds",
			zap.Float64("available", insufficientFunds.Available),
			zap.Float64("required", insufficientFunds.Required),
		)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.As(err, &limitExceeded):
		logger.Warn("limit exceeded", zap.String("error", err.Error()))
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.As(err, &duplicate):
		logger.Debug("duplicate resource", zap.String("error", err.Error()))
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &forbidden):
		logger.Warn("forbidden access", zap.String("error", err.Error()))
		writeError(w, http.StatusForbidden, err.Error())
	case errors.As(err, &invalidBarcode):
		logger.Debug("invalid barcode", zap.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &unauthorized):
		logger.Warn("unauthorized", zap.String("error", err.Error()))
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.As(err, &accountBlocked):
		logger.Warn("account blocked", zap.String("status", accountBlocked.Status))
		writeError(w, http.StatusForbidden, err.Error())
	case errors.As(err, &conflict):
		logger.Debug("conflict", zap.String("error", err.Error()))
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &invalidCode):
		logger.Warn("invalid verification code")
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		logger.Error("unhandled error", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
