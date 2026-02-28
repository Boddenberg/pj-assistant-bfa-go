package handler

import (
	"encoding/json"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ============================================================
// PIX Keys — lookup, register, list, delete
// ============================================================

func pixKeyLookupHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/pix/keys/lookup")
		defer span.End()

		keyValue := r.URL.Query().Get("key")
		keyType := r.URL.Query().Get("keyType")
		if keyType == "" {
			keyType = r.URL.Query().Get("type") // alias: ?type=email
		}

		pixKey, err := bankSvc.LookupPixKey(ctx, keyType, keyValue)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		// Resolve the customer profile + account for the recipient display
		recipientName, recipientDoc, recipientBank, recipientBranch, recipientAcct, lookupErr := bankSvc.GetCustomerLookupData(ctx, pixKey.CustomerID)
		if lookupErr != nil {
			logger.Warn("could not resolve recipient data", zap.String("customer_id", pixKey.CustomerID), zap.Error(lookupErr))
			recipientName = "Destinatário"
			recipientBank = "Itaú Unibanco"
		}

		resp := domain.PixKeyLookupResponse{
			KeyType: pixKey.KeyType,
			Recipient: &domain.PixRecipient{
				Name:     recipientName,
				Document: recipientDoc,
				Bank:     recipientBank,
				Branch:   recipientBranch,
				Account:  recipientAcct,
				PixKey: &domain.PixKeyInfo{
					Type:  pixKey.KeyType,
					Value: pixKey.KeyValue,
				},
			},
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func pixKeyRegisterHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/keys/register")
		defer span.End()

		var req domain.PixKeyRegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.RegisterPixKey(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func pixKeyDeleteByValueHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /v1/pix/keys")
		defer span.End()

		var req struct {
			CustomerID string `json:"customerId"`
			KeyType    string `json:"keyType"`
			KeyValue   string `json:"keyValue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.CustomerID == "" || req.KeyType == "" || req.KeyValue == "" {
			writeError(w, http.StatusBadRequest, "customerId, keyType and keyValue are required")
			return
		}

		if err := svc.DeletePixKeyByValue(ctx, req.CustomerID, req.KeyType, req.KeyValue); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Chave Pix removida com sucesso."})
	}
}

func listPixKeysHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /pix/keys")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		keys, err := svc.ListPixKeys(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		if keys == nil {
			keys = []domain.PixKey{}
		}
		type pixKeyDisplay struct {
			domain.PixKey
			FormattedValue string `json:"formatted_value"`
		}
		result := make([]pixKeyDisplay, len(keys))
		for i, k := range keys {
			result[i] = pixKeyDisplay{
				PixKey:         k,
				FormattedValue: formatKeyValue(k.KeyType, k.KeyValue),
			}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func deletePixKeyHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /pix/keys/{keyId}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		keyID := chi.URLParam(r, "keyId")
		if err := svc.DeletePixKey(ctx, customerID, keyID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Chave Pix excluída com sucesso"})
	}
}

func creditLimitHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-limit")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		limit, err := svc.GetCreditLimit(ctx, customerID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"creditLimit": 0})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"creditLimit": limit})
	}
}
