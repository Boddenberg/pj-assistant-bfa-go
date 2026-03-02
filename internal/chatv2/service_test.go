package chatv2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// ============================================================
// Helper: mock Agent Python server
// ============================================================

func mockAgentServer(response AgentResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func newTestService(agentURL string) *Service {
	logger := zap.NewNop()
	client := NewClient(agentURL, 5*1e9, logger) // 5s
	sessions := NewSessionStore()
	repo := NewInMemoryAccountRepository(logger)
	return NewService(client, sessions, repo, logger)
}

// ============================================================
// Test: CNPJ validation
// ============================================================

func TestValidateCNPJ_Valid(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cnpjValidator{repo: repo}

	if err := v.Validate("12.345.678/0001-90", nil); err != nil {
		t.Errorf("expected valid CNPJ, got error: %v", err)
	}
}

func TestValidateCNPJ_TooShort(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cnpjValidator{repo: repo}

	if err := v.Validate("123456", nil); err == nil {
		t.Error("expected error for short CNPJ")
	}
}

func TestValidateCNPJ_AlreadyExists(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	repo.cnpjs["12345678000190"] = true
	v := &cnpjValidator{repo: repo}

	if err := v.Validate("12345678000190", nil); err == nil {
		t.Error("expected error for duplicate CNPJ")
	}
}

// ============================================================
// Test: Password confirmation
// ============================================================

func TestValidatePasswordConfirmation_Match(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{"password": "123456"}}

	if err := v.Validate("123456", session); err != nil {
		t.Errorf("expected match, got error: %v", err)
	}
}

func TestValidatePasswordConfirmation_Mismatch(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{"password": "123456"}}

	if err := v.Validate("654321", session); err == nil {
		t.Error("expected error for mismatched password confirmation")
	}
}

func TestValidatePasswordConfirmation_NoPasswordInSession(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{}}

	if err := v.Validate("123456", session); err == nil {
		t.Error("expected error when password not in session")
	}
}

// ============================================================
// Test: Birth date (18+)
// ============================================================

func TestValidateBirthDate_Valid(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate("15/06/1990", nil); err != nil {
		t.Errorf("expected valid date, got error: %v", err)
	}
}

func TestValidateBirthDate_TooYoung(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate("01/01/2020", nil); err == nil {
		t.Error("expected error for under-18")
	}
}

func TestValidateBirthDate_InvalidFormat(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate("1990-06-15", nil); err == nil {
		t.Error("expected error for wrong format")
	}
}

// ============================================================
// Test: Password (6 digits)
// ============================================================

func TestValidatePassword_Valid(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate("123456", nil); err != nil {
		t.Errorf("expected valid password, got error: %v", err)
	}
}

func TestValidatePassword_NotDigits(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate("abc123", nil); err == nil {
		t.Error("expected error for non-digit password")
	}
}

func TestValidatePassword_WrongLength(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate("12345", nil); err == nil {
		t.Error("expected error for wrong length password")
	}
}

// ============================================================
// Test: Email
// ============================================================

func TestValidateEmail_Valid(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate("joao@empresa.com", nil); err != nil {
		t.Errorf("expected valid email, got error: %v", err)
	}
}

func TestValidateEmail_NoAt(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate("joaoempresa.com", nil); err == nil {
		t.Error("expected error for email without @")
	}
}

// ============================================================
// Test: CPF
// ============================================================

func TestValidateCPF_Valid(t *testing.T) {
	v := &cpfValidator{}
	if err := v.Validate("123.456.789-01", nil); err != nil {
		t.Errorf("expected valid CPF, got error: %v", err)
	}
}

func TestValidateCPF_TooShort(t *testing.T) {
	v := &cpfValidator{}
	if err := v.Validate("12345", nil); err == nil {
		t.Error("expected error for short CPF")
	}
}

// ============================================================
// Test: Phone
// ============================================================

func TestValidatePhone_Valid(t *testing.T) {
	v := &phoneValidator{}
	if err := v.Validate("(11) 99999-1234", nil); err != nil {
		t.Errorf("expected valid phone, got error: %v", err)
	}
}

func TestValidatePhone_TooShort(t *testing.T) {
	v := &phoneValidator{}
	if err := v.Validate("12345", nil); err == nil {
		t.Error("expected error for short phone")
	}
}

// ============================================================
// Test: Welcome step (integration with mock agent)
// ============================================================

func TestProcessTurn_Welcome(t *testing.T) {
	agentResp := AgentResponse{
		Answer:   "Olá! Vou te ajudar a abrir sua conta PJ.",
		Context:  strPtr("onboarding"),
		Step:     strPtr("welcome"),
		NextStep: strPtr("cnpj"),
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-1", "Quero abrir conta PJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer != agentResp.Answer {
		t.Errorf("expected answer %q, got %q", agentResp.Answer, resp.Answer)
	}

	session := svc.sessions.Get("cust-1")
	if len(session.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(session.History))
	}

	// welcome → step=nil, validated=nil
	entry := session.History[0]
	if entry.Step != nil {
		t.Errorf("welcome step should be nil, got %v", *entry.Step)
	}
	if entry.Validated != nil {
		t.Errorf("welcome validated should be nil, got %v", *entry.Validated)
	}
}

// ============================================================
// Test: step == next_step (agent inline rejection)
// ============================================================

func TestProcessTurn_InlineRejection(t *testing.T) {
	agentResp := AgentResponse{
		Answer:   "CNPJ inválido, tente novamente.",
		Context:  strPtr("onboarding"),
		Step:     strPtr("cnpj"),
		NextStep: strPtr("cnpj"), // same → inline rejection
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-2", "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer != agentResp.Answer {
		t.Errorf("expected answer %q, got %q", agentResp.Answer, resp.Answer)
	}

	session := svc.sessions.Get("cust-2")
	entry := session.History[0]
	if entry.Step == nil || *entry.Step != "cnpj" {
		t.Error("expected step=cnpj for inline rejection")
	}
	if entry.Validated == nil || *entry.Validated != false {
		t.Error("expected validated=false for inline rejection")
	}
}

// ============================================================
// Test: Valid field → saved in session
// ============================================================

func TestProcessTurn_ValidField(t *testing.T) {
	agentResp := AgentResponse{
		Answer:     "Ótimo! Agora informe a Razão Social.",
		Context:    strPtr("onboarding"),
		Step:       strPtr("cnpj"),
		FieldValue: strPtr("12345678000190"),
		NextStep:   strPtr("razaoSocial"),
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-3", "12345678000190")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer != agentResp.Answer {
		t.Errorf("expected answer %q, got %q", agentResp.Answer, resp.Answer)
	}

	session := svc.sessions.Get("cust-3")
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Errorf("expected cnpj saved, got %q", session.OnboardingData["cnpj"])
	}

	entry := session.History[0]
	if entry.Validated == nil || *entry.Validated != true {
		t.Error("expected validated=true for valid cnpj")
	}
}

// ============================================================
// Test: null step → normal conversation
// ============================================================

func TestProcessTurn_NullStep(t *testing.T) {
	agentResp := AgentResponse{
		Answer:  "Posso ajudar com informações sobre contas PJ.",
		Context: nil,
		Step:    nil,
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-4", "O que vocês oferecem?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer != agentResp.Answer {
		t.Errorf("expected answer %q, got %q", agentResp.Answer, resp.Answer)
	}

	session := svc.sessions.Get("cust-4")
	entry := session.History[0]
	if entry.Step != nil {
		t.Error("expected step=nil for normal conversation")
	}
	if entry.Validated != nil {
		t.Error("expected validated=nil for normal conversation")
	}
}
