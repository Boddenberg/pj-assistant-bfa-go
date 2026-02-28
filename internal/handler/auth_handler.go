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
// 9. Autenticação
// ============================================================

func authRegisterHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/register")
		defer span.End()

		var req domain.RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Register(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func authLoginHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/login")
		defer span.End()

		var req domain.LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Login(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authRefreshHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/refresh")
		defer span.End()

		var req domain.RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Refresh(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authLogoutHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/logout")
		defer span.End()

		customerID := CustomerIDFromContext(ctx)
		if customerID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if err := authSvc.Logout(ctx, customerID); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func authPasswordResetRequestHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/password/reset-request")
		defer span.End()

		var req domain.PasswordResetRequestBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.PasswordResetRequest(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authPasswordResetConfirmHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/password/reset-confirm")
		defer span.End()

		var req domain.PasswordResetConfirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := authSvc.PasswordResetConfirm(ctx, &req); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "Senha redefinida com sucesso"})
	}
}

func authChangePasswordHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/auth/password")
		defer span.End()

		customerID := CustomerIDFromContext(ctx)
		if customerID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req domain.ChangePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := authSvc.ChangePassword(ctx, customerID, &req); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "Senha alterada com sucesso"})
	}
}

// ============================================================
// 10. Profile & Representative
// ============================================================

func updateProfileHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/customers/{customerId}/profile")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		var req domain.UpdateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.UpdateProfile(ctx, customerID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func updateRepresentativeHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/customers/{customerId}/representative")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		var req domain.UpdateRepresentativeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.UpdateRepresentative(ctx, customerID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
