// Package port — chat_port.go define a interface (port) para o client
// que se comunica com o Agent Python via POST /v1/chat.
//
// Seguindo a arquitetura hexagonal, o ChatService depende dessa interface
// e NÃO do client concreto. Isso facilita testes e troca de implementação.
package port

import (
	"context"

	chatdomain "github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
)

// ChatAgentCaller é a interface para enviar mensagens ao Agent Python.
// O client concreto (ChatAgentClient) implementa essa interface.
//
// Por que uma interface separada do AgentCaller?
//   - AgentCaller chama POST /v1/agent/invoke (payload pesado, legado)
//   - ChatAgentCaller chama POST /v1/chat (payload leve, novo)
type ChatAgentCaller interface {
	SendChat(ctx context.Context, req *chatdomain.ChatAgentRequest) (*chatdomain.ChatAgentResponse, error)
}
