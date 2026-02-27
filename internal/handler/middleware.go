package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"
	"go.uber.org/zap"
)

type contextKey string

const customerIDKey contextKey = "customerID"

// JWTAuthMiddleware validates Bearer tokens and injects customerID into context.
func JWTAuthMiddleware(authSvc *service.AuthService, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn("auth: missing token",
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
				)
				writeError(w, http.StatusUnauthorized, "Token de autenticação não fornecido")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				logger.Warn("auth: invalid token format",
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
				)
				writeError(w, http.StatusUnauthorized, "Formato de token inválido")
				return
			}

			tokenString := parts[1]
			claims, err := authSvc.ValidateAccessToken(tokenString)
			if err != nil {
				logger.Warn("auth: invalid or expired token",
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
					zap.Error(err),
				)
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}

			// Inject customerID into context
			ctx := context.WithValue(r.Context(), customerIDKey, claims.Sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CustomerIDFromContext extracts the authenticated customer ID from context.
func CustomerIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(customerIDKey).(string)
	return v
}
