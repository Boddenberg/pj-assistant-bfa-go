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

var ctx = context.Background()

// ============================================================
// Test: CNPJ validation
// ============================================================

func TestValidateCNPJ_Valid(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cnpjValidator{repo: repo}
	if err := v.Validate(ctx, "12.345.678/0001-90", nil); err != nil {
		t.Errorf("expected valid CNPJ, got error: %v", err)
	}
}

func TestValidateCNPJ_TooShort(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cnpjValidator{repo: repo}
	if err := v.Validate(ctx, "123456", nil); err == nil {
		t.Error("expected error for short CNPJ")
	}
}

func TestValidateCNPJ_AlreadyExists(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	repo.cnpjs["12345678000190"] = true
	v := &cnpjValidator{repo: repo}
	if err := v.Validate(ctx, "12345678000190", nil); err == nil {
		t.Error("expected error for duplicate CNPJ")
	}
}

// ============================================================
// Test: Password confirmation
// ============================================================

func TestValidatePasswordConfirmation_Match(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{"password": "123456"}}
	if err := v.Validate(ctx, "123456", session); err != nil {
		t.Errorf("expected match, got error: %v", err)
	}
}

func TestValidatePasswordConfirmation_Mismatch(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{"password": "123456"}}
	if err := v.Validate(ctx, "654321", session); err == nil {
		t.Error("expected error for mismatched password confirmation")
	}
}

func TestValidatePasswordConfirmation_NoPasswordInSession(t *testing.T) {
	v := &passwordConfirmationValidator{}
	session := &Session{OnboardingData: map[string]string{}}
	if err := v.Validate(ctx, "123456", session); err == nil {
		t.Error("expected error when password not in session")
	}
}

// ============================================================
// Test: Birth date (18+)
// ============================================================

func TestValidateBirthDate_Valid(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate(ctx, "15/06/1990", nil); err != nil {
		t.Errorf("expected valid date, got error: %v", err)
	}
}

func TestValidateBirthDate_TooYoung(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate(ctx, "01/01/2020", nil); err == nil {
		t.Error("expected error for under-18")
	}
}

func TestValidateBirthDate_InvalidFormat(t *testing.T) {
	v := &birthDateValidator{}
	if err := v.Validate(ctx, "1990-06-15", nil); err == nil {
		t.Error("expected error for wrong format")
	}
}

// ============================================================
// Test: Password (6 digits)
// ============================================================

func TestValidatePassword_Valid(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate(ctx, "123456", nil); err != nil {
		t.Errorf("expected valid password, got error: %v", err)
	}
}

func TestValidatePassword_NotDigits(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate(ctx, "abc123", nil); err == nil {
		t.Error("expected error for non-digit password")
	}
}

func TestValidatePassword_WrongLength(t *testing.T) {
	v := &passwordValidator{}
	if err := v.Validate(ctx, "12345", nil); err == nil {
		t.Error("expected error for wrong length password")
	}
}

// ============================================================
// Test: Email
// ============================================================

func TestValidateEmail_Valid(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate(ctx, "joao@empresa.com", nil); err != nil {
		t.Errorf("expected valid email, got error: %v", err)
	}
}

func TestValidateEmail_NoAt(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate(ctx, "joaoempresa.com", nil); err == nil {
		t.Error("expected error for email without @")
	}
}

func TestValidateEmail_DoubleAt(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate(ctx, "filipe@filipe@cilipe", nil); err == nil {
		t.Error("expected error for email with double @")
	}
}

func TestValidateEmail_ValidDotCom(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate(ctx, "filipe@filipe.com", nil); err != nil {
		t.Errorf("expected valid email, got error: %v", err)
	}
}

func TestValidateEmail_ValidDotComBr(t *testing.T) {
	v := &emailValidator{}
	if err := v.Validate(ctx, "contato@empresa.com.br", nil); err != nil {
		t.Errorf("expected valid email, got error: %v", err)
	}
}

// ============================================================
// Test: CPF
// ============================================================

func TestValidateCPF_Valid(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cpfValidator{repo: repo}
	if err := v.Validate(ctx, "123.456.789-01", nil); err != nil {
		t.Errorf("expected valid CPF, got error: %v", err)
	}
}

func TestValidateCPF_TooShort(t *testing.T) {
	repo := NewInMemoryAccountRepository(zap.NewNop())
	v := &cpfValidator{repo: repo}
	if err := v.Validate(ctx, "12345", nil); err == nil {
		t.Error("expected error for short CPF")
	}
}

// ============================================================
// Test: Phone
// ============================================================

func TestValidatePhone_Valid(t *testing.T) {
	v := &phoneValidator{}
	if err := v.Validate(ctx, "(11) 99999-1234", nil); err != nil {
		t.Errorf("expected valid phone, got error: %v", err)
	}
}

func TestValidatePhone_TooShort(t *testing.T) {
	v := &phoneValidator{}
	if err := v.Validate(ctx, "12345", nil); err == nil {
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

	// BFA deve ter setado ExpectedStep = "cnpj"
	if session.ExpectedStep != "cnpj" {
		t.Errorf("expected ExpectedStep=cnpj after welcome, got %q", session.ExpectedStep)
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
	// Pre-set session as if welcome already happened
	session := svc.sessions.Get("cust-2")
	session.ExpectedStep = "cnpj"

	resp, err := svc.ProcessTurn(context.Background(), "cust-2", "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BFA should reject "123" (only 3 digits, needs 14)
	if resp.Answer == "" {
		t.Error("expected non-empty answer")
	}

	_ = resp
	session = svc.sessions.Get("cust-2")
	// Should still be on cnpj step
	if session.ExpectedStep != "cnpj" {
		t.Errorf("expected ExpectedStep=cnpj after rejection, got %q", session.ExpectedStep)
	}
	if session.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", session.Retries)
	}
}

// ============================================================
// Test: Valid field → saved in session
// ============================================================

func TestProcessTurn_ValidField(t *testing.T) {
	// BFA calls agent twice: (1) initial call, (2) advance call after validation
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp AgentResponse
		if callCount == 1 {
			resp = AgentResponse{
				Answer:     "Ótimo! CNPJ recebido.",
				Context:    strPtr("onboarding"),
				Step:       strPtr("cnpj"),
				FieldValue: strPtr("12345678000190"),
				NextStep:   strPtr("razaoSocial"),
			}
		} else {
			resp = AgentResponse{
				Answer:   "Agora informe a Razão Social.",
				Context:  strPtr("onboarding"),
				Step:     strPtr("cnpj"),
				NextStep: strPtr("razaoSocial"),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)
	// Pre-set session as if welcome already happened
	session := svc.sessions.Get("cust-3")
	session.ExpectedStep = "cnpj"

	resp, err := svc.ProcessTurn(context.Background(), "cust-3", "12345678000190")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer == "" {
		t.Error("expected non-empty answer")
	}

	session = svc.sessions.Get("cust-3")
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Errorf("expected cnpj saved, got %q", session.OnboardingData["cnpj"])
	}

	// BFA should have advanced to razaoSocial
	if session.ExpectedStep != "razaoSocial" {
		t.Errorf("expected ExpectedStep=razaoSocial, got %q", session.ExpectedStep)
	}

	if session.Retries != 0 {
		t.Errorf("expected retries=0 after valid field, got %d", session.Retries)
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

// ============================================================
// Test: Full onboarding flow — reproduces the exact bugs found
// ============================================================

func TestProcessTurn_FullOnboarding_NoCrossContamination(t *testing.T) {
	// This test reproduces the scenario where:
	// 1. User sends valid CNPJ → should save as cnpj, advance to razaoSocial
	// 2. User sends valid razaoSocial → should save as razaoSocial, advance to nomeFantasia
	// 3. User sends "filipe@filipe@cilipe" as nomeFantasia → should save as nomeFantasia (valid, min 2 chars)
	// 4. User sends "filipe@filipe@cilipe" as email → should be REJECTED (invalid email)
	// 5. User sends "filipe@filipe.com" as email → should save as email, advance
	//
	// PREVIOUSLY BROKEN: values would cross-contaminate between steps

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Agent always returns a generic response — BFA controls everything
		resp := AgentResponse{
			Answer:  "Processado pelo agente.",
			Context: strPtr("onboarding"),
			Step:    strPtr("cnpj"),
			NextStep: strPtr("razaoSocial"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)
	session := svc.sessions.Get("cust-flow")
	session.ExpectedStep = "cnpj" // simulate post-welcome

	// Step 1: Valid CNPJ
	_, err := svc.ProcessTurn(context.Background(), "cust-flow", "12.345.678/0001-90")
	if err != nil {
		t.Fatalf("cnpj turn failed: %v", err)
	}
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Errorf("cnpj not saved correctly, got %q", session.OnboardingData["cnpj"])
	}
	if session.ExpectedStep != "razaoSocial" {
		t.Fatalf("expected razaoSocial, got %q", session.ExpectedStep)
	}

	// Step 2: Valid razaoSocial
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "Empresa Teste LTDA")
	if err != nil {
		t.Fatalf("razaoSocial turn failed: %v", err)
	}
	if session.OnboardingData["razaoSocial"] != "Empresa Teste LTDA" {
		t.Errorf("razaoSocial not saved correctly, got %q", session.OnboardingData["razaoSocial"])
	}
	if session.ExpectedStep != "nomeFantasia" {
		t.Fatalf("expected nomeFantasia, got %q", session.ExpectedStep)
	}

	// Step 3: nomeFantasia — "filipe@filipe@cilipe" is valid (min 2 chars)
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe@cilipe")
	if err != nil {
		t.Fatalf("nomeFantasia turn failed: %v", err)
	}
	if session.OnboardingData["nomeFantasia"] != "filipe@filipe@cilipe" {
		t.Errorf("nomeFantasia not saved correctly, got %q", session.OnboardingData["nomeFantasia"])
	}
	if session.ExpectedStep != "email" {
		t.Fatalf("expected email, got %q", session.ExpectedStep)
	}

	// Step 4: email — "filipe@filipe@cilipe" should be REJECTED (double @)
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe@cilipe")
	if err != nil {
		t.Fatalf("email rejection turn failed: %v", err)
	}
	if _, ok := session.OnboardingData["email"]; ok {
		t.Error("email should NOT have been saved — double @ is invalid")
	}
	if session.ExpectedStep != "email" {
		t.Fatalf("should still be on email step, got %q", session.ExpectedStep)
	}

	// Step 5: email — valid email
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe.com")
	if err != nil {
		t.Fatalf("email valid turn failed: %v", err)
	}
	if session.OnboardingData["email"] != "filipe@filipe.com" {
		t.Errorf("email not saved correctly, got %q", session.OnboardingData["email"])
	}
	if session.ExpectedStep != "representanteName" {
		t.Fatalf("expected representanteName, got %q", session.ExpectedStep)
	}

	// Verify no cross-contamination: nomeFantasia should NOT contain the email
	if session.OnboardingData["nomeFantasia"] == "filipe@filipe.com" {
		t.Error("CROSS-CONTAMINATION: nomeFantasia was overwritten with email value!")
	}
}

// ============================================================
// Test: CPF-like input saved as representanteName (previously broken)
// ============================================================

func TestProcessTurn_CPFAsName_Rejected(t *testing.T) {
	// "072.187.010-4" has >= 3 chars so passes minLen(3) for representanteName
	// This is technically valid for the validator, but we verify it's saved
	// in the correct field (representanteName), not representanteCpf
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AgentResponse{
			Answer:  "OK",
			Context: strPtr("onboarding"),
			Step:    strPtr("representanteName"),
			NextStep: strPtr("representanteCpf"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)
	session := svc.sessions.Get("cust-cpf")
	session.ExpectedStep = "representanteName"

	_, err := svc.ProcessTurn(context.Background(), "cust-cpf", "072.187.010-4")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	// Should be saved in representanteName (even though it looks like a CPF)
	if session.OnboardingData["representanteName"] != "072.187.010-4" {
		t.Errorf("representanteName not saved, got %q", session.OnboardingData["representanteName"])
	}

	// Should NOT be saved as representanteCpf
	if _, ok := session.OnboardingData["representanteCpf"]; ok {
		t.Error("representanteCpf should NOT be populated at this point")
	}

	// ExpectedStep should advance to representanteCpf
	if session.ExpectedStep != "representanteCpf" {
		t.Errorf("expected representanteCpf, got %q", session.ExpectedStep)
	}
}

// ============================================================
// Test: NextStepAfter helper
// ============================================================

func TestNextStepAfter(t *testing.T) {
	tests := []struct {
		current  string
		expected string
	}{
		{"cnpj", "razaoSocial"},
		{"razaoSocial", "nomeFantasia"},
		{"nomeFantasia", "email"},
		{"email", "representanteName"},
		{"password", "passwordConfirmation"},
		{"passwordConfirmation", "completed"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := NextStepAfter(tt.current)
		if got != tt.expected {
			t.Errorf("NextStepAfter(%q) = %q, want %q", tt.current, got, tt.expected)
		}
	}
}
