package domain

import "fmt"

// Error types for consistent error handling across the BFA.

// ErrNotFound indicates a resource was not found.
type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

// ErrExternalService indicates a failure in an external service call.
type ErrExternalService struct {
	Service string
	Err     error
}

func (e *ErrExternalService) Error() string {
	return fmt.Sprintf("external service error [%s]: %v", e.Service, e.Err)
}

func (e *ErrExternalService) Unwrap() error {
	return e.Err
}

// ErrTimeout indicates an operation exceeded its deadline.
type ErrTimeout struct {
	Operation string
}

func (e *ErrTimeout) Error() string {
	return fmt.Sprintf("operation timed out: %s", e.Operation)
}

// ErrCircuitOpen indicates the circuit breaker is open.
type ErrCircuitOpen struct {
	Service string
}

func (e *ErrCircuitOpen) Error() string {
	return fmt.Sprintf("circuit breaker open for service: %s", e.Service)
}

// ErrValidation indicates a validation error (bad input).
type ErrValidation struct {
	Field   string
	Message string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("validation error on '%s': %s", e.Field, e.Message)
}

// ErrInsufficientFunds indicates not enough balance for the operation.
type ErrInsufficientFunds struct {
	Available float64
	Required  float64
}

func (e *ErrInsufficientFunds) Error() string {
	return fmt.Sprintf("insufficient funds: available=%.2f required=%.2f", e.Available, e.Required)
}

// ErrLimitExceeded indicates a transaction limit was exceeded.
type ErrLimitExceeded struct {
	LimitType string
	Limit     float64
	Current   float64
}

func (e *ErrLimitExceeded) Error() string {
	return fmt.Sprintf("limit exceeded [%s]: limit=%.2f current=%.2f", e.LimitType, e.Limit, e.Current)
}

// ErrDuplicate indicates a duplicate operation (idempotency check).
type ErrDuplicate struct {
	Key string
}

func (e *ErrDuplicate) Error() string {
	return fmt.Sprintf("duplicate operation: %s", e.Key)
}

// ErrForbidden indicates the user lacks permission for the operation.
type ErrForbidden struct {
	Action string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: %s", e.Action)
}

// ErrInvalidBarcode indicates an invalid barcode or digitable line.
type ErrInvalidBarcode struct {
	Input  string
	Reason string
}

func (e *ErrInvalidBarcode) Error() string {
	return fmt.Sprintf("invalid barcode/digitable line: %s", e.Reason)
}

// ErrUnauthorized indicates invalid credentials or token.
type ErrUnauthorized struct {
	Message string
}

func (e *ErrUnauthorized) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "unauthorized"
}

// ErrAccountBlocked indicates the account is blocked.
type ErrAccountBlocked struct {
	Status string
}

func (e *ErrAccountBlocked) Error() string {
	return fmt.Sprintf("Conta bloqueada")
}

// ErrConflict indicates a resource already exists (e.g. duplicate CNPJ).
type ErrConflict struct {
	Message string
}

func (e *ErrConflict) Error() string {
	return e.Message
}

// ErrInvalidCode indicates an invalid or expired verification code.
type ErrInvalidCode struct{}

func (e *ErrInvalidCode) Error() string {
	return "Código inválido ou expirado"
}
