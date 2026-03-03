package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Validator valida o field_value de um step do onboarding.
type Validator interface {
	Validate(ctx context.Context, value string, session *Session) error
}

// ValidatorRegistry mapeia step → Validator.
// Preenchido em NewValidatorRegistry com os 10 campos do onboarding.
type ValidatorRegistry map[string]Validator

func NewValidatorRegistry(repo AccountRepository) ValidatorRegistry {
	return ValidatorRegistry{
		"cnpj":                   &cnpjValidator{repo: repo},
		"razaoSocial":            &minLenValidator{field: "Razão Social", min: 3},
		"nomeFantasia":           &minLenValidator{field: "Nome Fantasia", min: 2},
		"email":                  &emailValidator{},
		"representanteName":      &minLenValidator{field: "Nome do Representante", min: 3},
		"representanteCpf":       &cpfValidator{repo: repo},
		"representantePhone":     &phoneValidator{},
		"representanteBirthDate": &birthDateValidator{},
		"password":               &passwordValidator{},
		"passwordConfirmation":   &passwordConfirmationValidator{},
	}
}

/*
 * Implementações
 */

/* CNPJ: 14 dígitos + único */

type cnpjValidator struct {
	repo AccountRepository
}

func (v *cnpjValidator) Validate(ctx context.Context, value string, _ *Session) error {
	digits := onlyDigits(value)
	if len(digits) != 14 {
		return fmt.Errorf("CNPJ deve conter exatamente 14 dígitos (recebido: %d)", len(digits))
	}
	if v.repo.CNPJExists(ctx, digits) {
		return fmt.Errorf("CNPJ %s já está cadastrado no sistema", digits)
	}
	return nil
}

/* Texto com tamanho mínimo (razaoSocial, nomeFantasia, representanteName) */

type minLenValidator struct {
	field string
	min   int
}

func (v *minLenValidator) Validate(_ context.Context, value string, _ *Session) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < v.min {
		return fmt.Errorf("%s deve ter no mínimo %d caracteres", v.field, v.min)
	}
	return nil
}

/* Email: contém @ e .com */

type emailValidator struct{}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func (v *emailValidator) Validate(_ context.Context, value string, _ *Session) error {
	trimmed := strings.TrimSpace(value)
	if !emailRegex.MatchString(trimmed) {
		return fmt.Errorf("e-mail inválido: formato esperado usuario@dominio.com")
	}
	return nil
}

/* CPF: 11 dígitos + único */

type cpfValidator struct {
	repo AccountRepository
}

func (v *cpfValidator) Validate(ctx context.Context, value string, _ *Session) error {
	digits := onlyDigits(value)
	if len(digits) != 11 {
		return fmt.Errorf("CPF deve conter exatamente 11 dígitos (recebido: %d)", len(digits))
	}
	if v.repo.CPFExists(ctx, digits) {
		return fmt.Errorf("CPF %s já está cadastrado no sistema", digits)
	}
	return nil
}

/* Telefone: mínimo 10 dígitos */

type phoneValidator struct{}

func (v *phoneValidator) Validate(_ context.Context, value string, _ *Session) error {
	digits := onlyDigits(value)
	if len(digits) < 10 {
		return fmt.Errorf("telefone deve ter no mínimo 10 dígitos (recebido: %d)", len(digits))
	}
	return nil
}

/* Data de nascimento: DD/MM/AAAA, 18+ */

type birthDateValidator struct{}

var dateRegex = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)

func (v *birthDateValidator) Validate(_ context.Context, value string, _ *Session) error {
	trimmed := strings.TrimSpace(value)
	if !dateRegex.MatchString(trimmed) {
		return fmt.Errorf("data deve estar no formato DD/MM/AAAA")
	}

	parsed, err := time.Parse("02/01/2006", trimmed)
	if err != nil {
		return fmt.Errorf("data inválida: %s", trimmed)
	}

	age := time.Since(parsed)
	if age < 18*365*24*time.Hour {
		return fmt.Errorf("representante deve ter no mínimo 18 anos")
	}
	return nil
}

/* Senha: exatamente 6 dígitos numéricos */

type passwordValidator struct{}

func (v *passwordValidator) Validate(_ context.Context, value string, _ *Session) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 6 {
		return fmt.Errorf("senha deve ter exatamente 6 dígitos")
	}
	for _, r := range trimmed {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("senha deve conter apenas dígitos numéricos")
		}
	}
	return nil
}

/* Confirmação de senha: igual ao password salvo na sessão */

type passwordConfirmationValidator struct{}

func (v *passwordConfirmationValidator) Validate(_ context.Context, value string, session *Session) error {
	saved, ok := session.OnboardingData["password"]
	if !ok {
		return fmt.Errorf("senha original não encontrada na sessão")
	}
	if strings.TrimSpace(value) != saved {
		return fmt.Errorf("confirmação de senha não confere com a senha informada")
	}
	return nil
}

/*
 * Helpers
 */

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
