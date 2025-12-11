package protocols

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// WinRMCredentials represents credentials for Windows Remote Management
type WinRMCredentials struct {
	Username string `json:"username" validate:"required,min=1"`
	Password string `json:"password" validate:"required,min=1"`
	Domain   string `json:"domain,omitempty"`
	UseHTTPS bool   `json:"use_https"`
}

// SSHCredentials represents credentials for SSH access
type SSHCredentials struct {
	Username   string `json:"username" validate:"required,min=1"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Port       int    `json:"port,omitempty"`
}

// Validate implements custom validation for SSH credentials
// Either password or private_key must be provided
func (s *SSHCredentials) Validate() error {
	if s.Password == "" && s.PrivateKey == "" {
		return fmt.Errorf("either password or private_key is required for SSH")
	}
	return nil
}

// SNMPCredentials represents credentials for SNMP v2c access
type SNMPCredentials struct {
	Community string `json:"community" validate:"required,min=1"`
}

// Global validator instance
var validate = validator.New()

// ValidationError represents a field-level validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors holds multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// Error implements the error interface for ValidationErrors
func (v *ValidationErrors) Error() string {
	if len(v.Errors) == 0 {
		return "validation failed"
	}
	messages := make([]string, len(v.Errors))
	for i, e := range v.Errors {
		messages[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(messages, "; "))
}

// ValidateCredentialStruct validates any credential struct and returns detailed errors
func ValidateCredentialStruct(creds interface{}) error {
	err := validate.Struct(creds)
	if err != nil {
		validationErrs := &ValidationErrors{}
		for _, e := range err.(validator.ValidationErrors) {
			validationErrs.Errors = append(validationErrs.Errors, ValidationError{
				Field:   toSnakeCase(e.Field()),
				Message: formatValidationMessage(e),
			})
		}
		return validationErrs
	}

	// Check for custom Validate method
	if v, ok := creds.(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return &ValidationErrors{
				Errors: []ValidationError{{Field: "_custom", Message: err.Error()}},
			}
		}
	}
	return nil
}

// formatValidationMessage creates human-readable error messages
func formatValidationMessage(e validator.FieldError) string {
	field := toSnakeCase(e.Field())
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "min":
		if e.Kind().String() == "string" {
			return fmt.Sprintf("%s must be at least %s characters", field, e.Param())
		}
		return fmt.Sprintf("%s must be at least %s", field, e.Param())
	case "max":
		if e.Kind().String() == "string" {
			return fmt.Sprintf("%s must be at most %s characters", field, e.Param())
		}
		return fmt.Sprintf("%s must be at most %s", field, e.Param())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, e.Param())
	default:
		return fmt.Sprintf("%s failed %s validation", field, e.Tag())
	}
}

// toSnakeCase converts PascalCase/camelCase to snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteByte(byte(r + 'a' - 'A'))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
