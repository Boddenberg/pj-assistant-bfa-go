// Package handler — chat_handler.go implementa o handler das rotas
// POST /v1/chat/{customerId} e POST /v1/chat — a entrada do chat com IA.
//
// ============================================================
// DIFERENÇA ENTRE AS ROTAS DE ASSISTANT
// ============================================================
//
// POST /v1/assistant/{customerId}  →  rota "pesada" (legada)
//   - Busca profile + transactions do Supabase
//   - Manda tudo pro Agent (POST /v1/agent/invoke)
//   - Resposta com metadata completa (tokens, tools, reasoning)
//
// POST /v1/chat/{customerId}       →  rota "leve" (cliente autenticado)
// POST /v1/chat                    →  rota "leve" (anônimo / onboarding)
//   - Recebe body JSON: {"query": "..."}
//   - Usa Strategy Pattern para rotear o contexto
//   - Chama Agent Python (POST /v1/chat)
//   - Retorna apenas: {"answer": "..."}
//
// A rota POST /v1/chat/{customerId} é a que o frontend/chatbot deve usar.
// Usamos POST (e não GET) porque proxies (Railway, CloudFlare) removem
// o body de requisições GET, causando erro 400/500 em produção.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/service"
	maindomain "github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// tracer é o tracer OpenTelemetry para o módulo chat/handler.
var tracer = otel.Tracer("chat/handler")

// ============================================================
// ChatHandler — POST /v1/chat/{customerId}
// ============================================================

// ChatHandler retorna o http.HandlerFunc para a rota POST /v1/chat/{customerId}.
//
// Rotas:
//
//	POST /v1/chat/ab84533a-...  → cliente autenticado
//	POST /v1/chat               → cliente anônimo (ex: abertura de conta)
//
// Request:
//
//	Content-Type: application/json
//	Body: {"query": "Quero abrir uma conta PJ"}
//
// Response (200 OK):
//
//	{"answer": "Olá! Vou te ajudar a abrir sua conta PJ..."}
//
// O handler é fino — só faz validação básica e delega pro ChatService.
// Toda a lógica de negócio (intent detection, strategy routing, agent call)
// fica no service layer.
//
// NOTA: usamos POST em vez de GET porque proxies reversos (Railway, CloudFlare)
// removem o body de requisições GET, causando erro em produção.
func ChatHandler(chatSvc *service.ChatService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Inicia o span de tracing para essa rota
		ctx, span := tracer.Start(r.Context(), "POST /v1/chat")
		defer span.End()

		// Extrai o customerId da URL (opcional).
		// Se não vier, usa "anonymous" (ex: abertura de conta).
		customerID := chi.URLParam(r, "customerId")
		if customerID == "" {
			customerID = "anonymous"
		}
		span.SetAttributes(attribute.String("customer.id", customerID))

		// Decodifica o body — esperamos {"query": "..."}
		var req domain.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: expected {\"query\": \"your message\"}")
			return
		}

		// Valida que a query não está vazia
		if req.Query == "" {
			writeError(w, http.StatusBadRequest, "query is required")
			return
		}

		// Delega para o ChatService — ele faz intent detection,
		// strategy routing e chamada ao agent
		resp, err := chatSvc.ProcessMessage(ctx, customerID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		// Retorna somente {"answer": "..."}
		writeJSON(w, http.StatusOK, resp)
	}
}

// ============================================================
// Helpers — funções utilitárias do chat handler
// ============================================================

// writeJSON serializa data como JSON e escreve na response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError escreve uma resposta de erro padronizada.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleServiceError mapeia erros de domínio para HTTP status codes.
func handleServiceError(w http.ResponseWriter, err error, logger *zap.Logger) {
	switch e := err.(type) {
	case *maindomain.ErrExternalService:
		logger.Error("external service error", zap.String("service", e.Service), zap.Error(e.Err))
		writeError(w, http.StatusBadGateway, "external service unavailable: "+e.Service)
	case *maindomain.ErrNotFound:
		writeError(w, http.StatusNotFound, e.Error())
	case *maindomain.ErrValidation:
		writeError(w, http.StatusUnprocessableEntity, e.Error())
	default:
		logger.Error("unexpected error in chat handler", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
