package chatv2

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Handler retorna um http.HandlerFunc para POST /v2/chat e POST /v2/chat/{customerID}.
func Handler(svc *Service, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req FrontendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Warn("chatv2: invalid request body", zap.Error(err))
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		if req.Query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "query is required",
			})
			return
		}

		// customerID vem do path; se não vier, fica "anonymous"
		customerID := chi.URLParam(r, "customerID")
		if customerID == "" {
			customerID = "anonymous"
		}

		logger.Info("⬇️  request recebida do frontend",
			zap.String("customer_id", customerID),
			zap.String("query", req.Query),
		)

		resp, err := svc.ProcessTurn(r.Context(), customerID, req.Query)
		if err != nil {
			logger.Error("chatv2: process turn failed", zap.Error(err))
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "failed to process chat turn",
			})
			return
		}

		logger.Info("⬆️  response enviada ao frontend",
			zap.String("answer", truncate(resp.Answer, 120)),
			zap.Any("step", resp.Step),
			zap.Any("next_step", resp.NextStep),
			zap.Any("context", resp.Context),
		)

		writeJSON(w, http.StatusOK, resp)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
