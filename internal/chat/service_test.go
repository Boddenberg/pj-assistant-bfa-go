package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

/*
 * Helper: mock Agent Python server
 */

func mockAgentServer(response AgentResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func newTestService(agentURL string) *Service {
	logger := zap.NewNop()
	client := NewClient(agentURL, 5*time.Second, 2, 100*time.Millisecond, logger)
	sessions := NewSessionStore()
	repo := NewInMemoryAccountRepository(logger)
	transcripts := NewInMemoryTranscriptRepository(logger)
	evaluations := NewInMemoryEvaluationRepository(logger)
	return NewService(client, sessions, repo, transcripts, evaluations, nil, nil, true, logger)
}

var ctx = context.Background()

/*
 * Test: CNPJ validation
 */

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

/*
 * Test: Password confirmation
 */

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

/*
 * Test: Birth date (18+)
 */

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

/*
 * Test: Password (6 digits)
 */

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

/*
 * Test: Email
 */

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

/*
 * Test: CPF
 */

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

/*
 * Test: Phone
 */

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

/*
 * Test: Welcome step — agent decides, BFA passes through
 */

func TestProcessTurn_Welcome(t *testing.T) {
	agentResp := AgentResponse{
		Answer:   "Olá! Vou te ajudar a abrir sua conta PJ.",
		Context:  "onboarding",
		Step:     strPtr("welcome"),
		NextStep: strPtr("cnpj"),
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-1", "Quero abrir conta PJ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Answer != agentResp.Answer {
		t.Errorf("expected answer %q, got %q", agentResp.Answer, resp.Answer)
	}

	// BFA passes through step/next_step from agent
	if derefStr(resp.Step) != "welcome" {
		t.Errorf("expected step=welcome, got %q", derefStr(resp.Step))
	}
	if derefStr(resp.NextStep) != "cnpj" {
		t.Errorf("expected next_step=cnpj, got %q", derefStr(resp.NextStep))
	}

	session := svc.sessions.Get("cust-1")
	if len(session.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(session.History))
	}

	// welcome → step=nil, validated=nil in history
	entry := session.History[0]
	if entry.Step != nil {
		t.Errorf("welcome step should be nil, got %v", *entry.Step)
	}
	if entry.Validated != nil {
		t.Errorf("welcome validated should be nil, got %v", *entry.Validated)
	}
}

/*
 * Test: step == next_step (agent inline rejection)
 * BFA also validates and rejects → sends validation_error to agent
 */

func TestProcessTurn_InlineRejection(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++
		var resp AgentResponse
		if callCount == 1 {
			// First call: agent rejects inline (step == next_step)
			resp = AgentResponse{
				Answer:   "CNPJ inválido, tente novamente.",
				Context:  "onboarding",
				Step:     strPtr("cnpj"),
				NextStep: strPtr("cnpj"), // same → inline rejection
			}
		} else {
			// Second call: BFA sends validation_error, agent formats error
			resp = AgentResponse{
				Answer:   "O CNPJ precisa ter 14 dígitos. Tente novamente.",
				Context:  "onboarding",
				Step:     strPtr("cnpj"),
				NextStep: strPtr("cnpj"),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)

	resp, err := svc.ProcessTurn(context.Background(), "cust-2", "123", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BFA should reject "123" (only 3 digits, needs 14) and call agent again
	if resp.Answer == "" {
		t.Error("expected non-empty answer")
	}

	session := svc.sessions.Get("cust-2")
	// CNPJ should NOT be saved
	if _, ok := session.OnboardingData["cnpj"]; ok {
		t.Error("cnpj should NOT be saved — invalid value")
	}
	if session.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", session.Retries)
	}
}

/*
 * Test: Valid field — agent says step=cnpj, next_step=razaoSocial
 * BFA validates, saves, passes through agent response
 */

func TestProcessTurn_ValidField(t *testing.T) {
	// Agent says: step=cnpj, next_step=razaoSocial (accepted)
	agentResp := AgentResponse{
		Answer:     "Ótimo! CNPJ recebido. Agora informe a Razão Social.",
		Context:    "onboarding",
		Step:       strPtr("cnpj"),
		FieldValue: strPtr("12345678000190"),
		NextStep:   strPtr("razaoSocial"),
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)

	resp, err := svc.ProcessTurn(context.Background(), "cust-3", "12345678000190", false)
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

	// BFA passes through step/next_step from agent
	if derefStr(resp.Step) != "cnpj" {
		t.Errorf("expected step=cnpj, got %q", derefStr(resp.Step))
	}
	if derefStr(resp.NextStep) != "razaoSocial" {
		t.Errorf("expected next_step=razaoSocial, got %q", derefStr(resp.NextStep))
	}

	if session.Retries != 0 {
		t.Errorf("expected retries=0 after valid field, got %d", session.Retries)
	}
}

/*
 * Test: Agent accepted but BFA rejects (validation mismatch)
 * Agent says step=email, next_step=representanteName (accepted)
 * BFA rejects because "not-an-email" fails regex
 */

func TestProcessTurn_AgentAcceptedBFARejected(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++
		var resp AgentResponse
		if callCount == 1 {
			// Agent accepted (step != next_step)
			resp = AgentResponse{
				Answer:   "Email recebido!",
				Context:  "onboarding",
				Step:     strPtr("email"),
				NextStep: strPtr("representanteName"),
			}
		} else {
			// BFA sends validation_error, agent reformats
			resp = AgentResponse{
				Answer:   "O email informado é inválido. Use o formato nome@dominio.com.",
				Context:  "onboarding",
				Step:     strPtr("email"),
				NextStep: strPtr("email"),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)

	resp, err := svc.ProcessTurn(context.Background(), "cust-email", "not-an-email", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BFA rejected — agent reformatted the error
	if resp.Answer != "O email informado é inválido. Use o formato nome@dominio.com." {
		t.Errorf("expected error message from agent, got %q", resp.Answer)
	}

	session := svc.sessions.Get("cust-email")
	if _, ok := session.OnboardingData["email"]; ok {
		t.Error("email should NOT be saved — invalid value")
	}
	if session.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", session.Retries)
	}
}

/*
 * Test: null step → normal conversation (pass-through)
 */

func TestProcessTurn_NullStep(t *testing.T) {
	agentResp := AgentResponse{
		Answer:  "Posso ajudar com informações sobre contas PJ.",
		Context: "",
		Step:    nil,
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)
	resp, err := svc.ProcessTurn(context.Background(), "cust-4", "O que vocês oferecem?", false)
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

/*
 * Test: Full onboarding flow — agent drives, BFA only validates
 * Reproduces the exact cross-contamination bugs found in testing
 */

func TestProcessTurn_FullOnboarding_NoCrossContamination(t *testing.T) {
	// The agent decides the order: cnpj → razaoSocial → nomeFantasia → email → ...
	// BFA validates the query using the agent's step.

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++

		// Decode request to see validation_error
		var req AgentRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp AgentResponse
		switch {
		case callCount == 1: // cnpj accepted
			resp = AgentResponse{
				Answer:   "CNPJ recebido. Qual a Razão Social?",
				Context:  "onboarding",
				Step:     strPtr("cnpj"),
				NextStep: strPtr("razaoSocial"),
			}
		case callCount == 2: // razaoSocial accepted
			resp = AgentResponse{
				Answer:   "Razão Social recebida. Qual o Nome Fantasia?",
				Context:  "onboarding",
				Step:     strPtr("razaoSocial"),
				NextStep: strPtr("nomeFantasia"),
			}
		case callCount == 3: // nomeFantasia accepted
			resp = AgentResponse{
				Answer:   "Nome Fantasia recebido. Qual o email?",
				Context:  "onboarding",
				Step:     strPtr("nomeFantasia"),
				NextStep: strPtr("email"),
			}
		case callCount == 4: // email — agent accepts, but BFA will reject (double @)
			resp = AgentResponse{
				Answer:   "Email recebido.",
				Context:  "onboarding",
				Step:     strPtr("email"),
				NextStep: strPtr("representanteName"),
			}
		case callCount == 5: // BFA rejected email, sends validation_error
			resp = AgentResponse{
				Answer:   "Email inválido. Tente novamente no formato nome@dominio.com.",
				Context:  "onboarding",
				Step:     strPtr("email"),
				NextStep: strPtr("email"),
			}
		case callCount == 6: // valid email accepted
			resp = AgentResponse{
				Answer:   "Email recebido! Qual o nome do representante?",
				Context:  "onboarding",
				Step:     strPtr("email"),
				NextStep: strPtr("representanteName"),
			}
		default:
			resp = AgentResponse{Answer: "OK", Context: "onboarding"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)

	// Step 1: Valid CNPJ — agent says step=cnpj, next_step=razaoSocial
	_, err := svc.ProcessTurn(context.Background(), "cust-flow", "12.345.678/0001-90", false)
	if err != nil {
		t.Fatalf("cnpj turn failed: %v", err)
	}
	session := svc.sessions.Get("cust-flow")
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Errorf("cnpj not saved correctly, got %q", session.OnboardingData["cnpj"])
	}

	// Step 2: Valid razaoSocial — agent says step=razaoSocial, next_step=nomeFantasia
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "Empresa Teste LTDA", false)
	if err != nil {
		t.Fatalf("razaoSocial turn failed: %v", err)
	}
	if session.OnboardingData["razaoSocial"] != "Empresa Teste LTDA" {
		t.Errorf("razaoSocial not saved correctly, got %q", session.OnboardingData["razaoSocial"])
	}

	// Step 3: nomeFantasia — "filipe@filipe@cilipe" is valid (min 2 chars)
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe@cilipe", false)
	if err != nil {
		t.Fatalf("nomeFantasia turn failed: %v", err)
	}
	if session.OnboardingData["nomeFantasia"] != "filipe@filipe@cilipe" {
		t.Errorf("nomeFantasia not saved correctly, got %q", session.OnboardingData["nomeFantasia"])
	}

	// Step 4: email — "filipe@filipe@cilipe" → agent accepts but BFA rejects (double @)
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe@cilipe", false)
	if err != nil {
		t.Fatalf("email rejection turn failed: %v", err)
	}
	if _, ok := session.OnboardingData["email"]; ok {
		t.Error("email should NOT have been saved — double @ is invalid")
	}

	// Step 5: valid email
	_, err = svc.ProcessTurn(context.Background(), "cust-flow", "filipe@filipe.com", false)
	if err != nil {
		t.Fatalf("email valid turn failed: %v", err)
	}
	if session.OnboardingData["email"] != "filipe@filipe.com" {
		t.Errorf("email not saved correctly, got %q", session.OnboardingData["email"])
	}

	// Verify no cross-contamination
	if session.OnboardingData["nomeFantasia"] == "filipe@filipe.com" {
		t.Error("CROSS-CONTAMINATION: nomeFantasia was overwritten with email value!")
	}
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Error("CROSS-CONTAMINATION: cnpj was overwritten!")
	}
}

/*
 * Test: CPF-like input saved as representanteName
 * Agent says step=representanteName → BFA validates with representanteName validator
 */

func TestProcessTurn_CPFAsName_SavedCorrectly(t *testing.T) {
	// Agent says step=representanteName, next_step=representanteCpf
	agentResp := AgentResponse{
		Answer:   "Nome recebido. Agora informe o CPF.",
		Context:  "onboarding",
		Step:     strPtr("representanteName"),
		NextStep: strPtr("representanteCpf"),
	}
	server := mockAgentServer(agentResp)
	defer server.Close()

	svc := newTestService(server.URL)

	// "072.187.010-4" has >= 3 chars so passes minLen(3) for representanteName
	_, err := svc.ProcessTurn(context.Background(), "cust-cpf", "072.187.010-4", false)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	session := svc.sessions.Get("cust-cpf")

	// Should be saved in representanteName (BFA uses agent's step)
	if session.OnboardingData["representanteName"] != "072.187.010-4" {
		t.Errorf("representanteName not saved, got %q", session.OnboardingData["representanteName"])
	}

	// Should NOT be saved as representanteCpf
	if _, ok := session.OnboardingData["representanteCpf"]; ok {
		t.Error("representanteCpf should NOT be populated at this point")
	}
}

/*
 * Test: Inline rejection where BFA accepts (BFA overrides agent)
 * Agent says step==next_step (rejected), but BFA validates OK
 * → BFA saves, re-calls agent so it can advance
 */

func TestProcessTurn_InlineRejection_BFAAccepts(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++
		var resp AgentResponse
		if callCount == 1 {
			// Agent rejects inline: step == next_step
			resp = AgentResponse{
				Answer:   "CNPJ parece inválido.",
				Context:  "onboarding",
				Step:     strPtr("cnpj"),
				NextStep: strPtr("cnpj"),
			}
		} else {
			// BFA saved and re-called agent → agent now advances
			resp = AgentResponse{
				Answer:   "CNPJ aceito! Qual a Razão Social?",
				Context:  "onboarding",
				Step:     strPtr("cnpj"),
				NextStep: strPtr("razaoSocial"),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)

	resp, err := svc.ProcessTurn(context.Background(), "cust-override", "12345678000190", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BFA overrode agent rejection — should see the advance response
	if resp.Answer != "CNPJ aceito! Qual a Razão Social?" {
		t.Errorf("expected advance response, got %q", resp.Answer)
	}

	session := svc.sessions.Get("cust-override")
	if session.OnboardingData["cnpj"] != "12345678000190" {
		t.Errorf("cnpj not saved, got %q", session.OnboardingData["cnpj"])
	}
}

/*
 * Test: BFA override re-call must NOT leak query to next field
 * Reproduces exact bug: agent rejects razaoSocial after retries,
 * BFA accepts, re-calls agent with same query → agent uses it as nomeFantasia
 */

func TestProcessTurn_BFAOverride_NoQueryLeak(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++

		var req AgentRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp AgentResponse
		switch callCount {
		case 1:
			// Agent rejects razaoSocial inline (step == next_step)
			// This happens when agent sees retry history and thinks it's still wrong
			resp = AgentResponse{
				Answer:   "Razão Social inválida.",
				Context:  "onboarding",
				Step:     strPtr("razaoSocial"),
				NextStep: strPtr("razaoSocial"),
			}
		case 2:
			// BFA overrode rejection, re-calls agent with validation_error signal.
			// validation_error MUST contain "CAMPO_ACEITO_BFA" to signal override.
			if !strings.Contains(req.ValidationError, "CAMPO_ACEITO_BFA") {
				t.Errorf("re-call should have CAMPO_ACEITO_BFA signal, got %q", req.ValidationError)
			}
			// razaoSocial should be in collected_data
			found := false
			for _, item := range req.CollectedData {
				if item.Key == "razaoSocial" && item.Value == "adbsasd" {
					found = true
				}
			}
			if !found {
				t.Error("razaoSocial should be in collected_data on re-call")
			}

			// Agent sees razaoSocial in collected_data, advances to nomeFantasia
			resp = AgentResponse{
				Answer:   "Razão Social recebida! Qual o Nome Fantasia?",
				Context:  "onboarding",
				Step:     strPtr("razaoSocial"),
				NextStep: strPtr("nomeFantasia"),
			}
		default:
			resp = AgentResponse{Answer: "OK", Context: "onboarding"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)
	// Pre-populate cnpj as if it was already collected
	session := svc.sessions.Get("cust-leak")
	session.OnboardingData["cnpj"] = "38535631000199"

	resp, err := svc.ProcessTurn(context.Background(), "cust-leak", "adbsasd", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// razaoSocial should be saved
	if session.OnboardingData["razaoSocial"] != "adbsasd" {
		t.Errorf("razaoSocial not saved, got %q", session.OnboardingData["razaoSocial"])
	}

	// nomeFantasia should NOT be saved — the re-call must not consume the query
	if _, ok := session.OnboardingData["nomeFantasia"]; ok {
		t.Errorf("QUERY LEAKED: nomeFantasia should NOT be populated, got %q", session.OnboardingData["nomeFantasia"])
	}

	// Response should be the advance response
	if resp.Answer != "Razão Social recebida! Qual o Nome Fantasia?" {
		t.Errorf("expected advance response, got %q", resp.Answer)
	}
}

/*
 * Test: sanitizeAnswer strips CAMPO_ACEITO_BFA from answer
 */

func TestSanitizeAnswer_RemovesCampoAceitoBFA(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "only CAMPO_ACEITO_BFA line",
			input:  "CAMPO_ACEITO_BFA: o campo 'cnpj' foi validado e salvo pelo BFA. Avance para o próximo campo.",
			expect: "Dado recebido! Continuando...",
		},
		{
			name:   "mixed with real answer",
			input:  "CAMPO_ACEITO_BFA: o campo 'cnpj' salvo.\nQual o Nome Fantasia da empresa?",
			expect: "Qual o Nome Fantasia da empresa?",
		},
		{
			name:   "no CAMPO_ACEITO_BFA",
			input:  "Perfeito! Qual o seu CNPJ?",
			expect: "Perfeito! Qual o seu CNPJ?",
		},
		{
			name:   "empty after removal",
			input:  "  CAMPO_ACEITO_BFA: salvo  ",
			expect: "Dado recebido! Continuando...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeAnswer(tc.input)
			if got != tc.expect {
				t.Errorf("sanitizeAnswer(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

/*
 * Test: MaxRetries exceeded triggers reset
 */

func TestProcessTurn_MaxRetries_Reset(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evaluate" {
			w.WriteHeader(http.StatusOK)
			return
		}
		callCount++
		// Agent always rejects inline (step == next_step)
		resp := AgentResponse{
			Answer:   "Telefone inválido.",
			Context:  "onboarding",
			Step:     strPtr("representantePhone"),
			NextStep: strPtr("representantePhone"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	svc := newTestService(server.URL)
	session := svc.sessions.Get("cust-reset")
	session.OnboardingData["cnpj"] = "38535631000199"
	session.OnboardingData["razaoSocial"] = "Empresa X"
	session.LastStep = "representantePhone"

	// Fail 3 times (MaxRetries = 3)
	var resp *FrontendResponse
	var err error
	for i := 0; i < 3; i++ {
		resp, err = svc.ProcessTurn(context.Background(), "cust-reset", "abc", false)
		if err != nil {
			t.Fatalf("unexpected error on attempt %d: %v", i+1, err)
		}
	}

	// After 3 failures, should get reset step
	if derefStr(resp.Step) != "reset" {
		t.Errorf("expected step=reset after MaxRetries, got %q", derefStr(resp.Step))
	}
	if derefStr(resp.NextStep) != "reset" {
		t.Errorf("expected next_step=reset after MaxRetries, got %q", derefStr(resp.NextStep))
	}

	// Session should be cleared
	newSession := svc.sessions.Get("cust-reset")
	if len(newSession.OnboardingData) != 0 {
		t.Errorf("session should be empty after reset, got %d fields", len(newSession.OnboardingData))
	}
}

/*
 * Test: Agent sends step=reset → BFA clears session
 */

func TestProcessTurn_AgentResetStep(t *testing.T) {
	server := mockAgentServer(AgentResponse{
		Answer:   "Vamos recomeçar do zero!",
		Context:  "onboarding",
		Step:     strPtr("reset"),
		NextStep: strPtr("reset"),
	})
	defer server.Close()

	svc := newTestService(server.URL)
	session := svc.sessions.Get("cust-agent-reset")
	session.OnboardingData["cnpj"] = "38535631000199"
	session.OnboardingData["razaoSocial"] = "Empresa Y"

	resp, err := svc.ProcessTurn(context.Background(), "cust-agent-reset", "resetar", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if derefStr(resp.Step) != "reset" {
		t.Errorf("expected step=reset, got %q", derefStr(resp.Step))
	}
	if resp.Answer != "Vamos recomeçar do zero!" {
		t.Errorf("expected agent's reset answer, got %q", resp.Answer)
	}

	// Session should be cleared
	newSession := svc.sessions.Get("cust-agent-reset")
	if len(newSession.OnboardingData) != 0 {
		t.Errorf("session should be empty after reset, got %d fields", len(newSession.OnboardingData))
	}
}
