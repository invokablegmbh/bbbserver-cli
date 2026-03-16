package api

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ExitCodeOK         = 0
	ExitCodeGeneral    = 1
	ExitCodeUsage      = 2
	ExitCodeAuth       = 3
	ExitCodeNotFound   = 4
	ExitCodeServer     = 5
	ExitCodeNetwork    = 6
	TypeValidation     = "validation_error"
	TypeAuth           = "auth_error"
	TypeNotFound       = "not_found"
	TypeServer         = "server_error"
	TypeNetwork        = "network_error"
	TypeAPI            = "api_error"
	TypeGeneral        = "general_error"
)

type APIError struct {
	Message   string
	Type      string
	Status    int
	RequestID string
	Err       error
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Status > 0 {
		return fmt.Sprintf("%s (status %d)", e.Message, e.Status)
	}
	return e.Message
}

func (e *APIError) Unwrap() error { return e.Err }

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type NetworkError struct {
	Message string
	Err     error
	Timeout bool
}

func (e *NetworkError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *NetworkError) Unwrap() error { return e.Err }

func ExitCode(err error) int {
	if err == nil {
		return ExitCodeOK
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return ExitCodeUsage
	}

	var networkErr *NetworkError
	if errors.As(err, &networkErr) {
		return ExitCodeNetwork
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 401 || apiErr.Status == 403:
			return ExitCodeAuth
		case apiErr.Status == 404:
			return ExitCodeNotFound
		case apiErr.Status >= 500:
			return ExitCodeServer
		default:
			return ExitCodeGeneral
		}
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unknown command") ||
		strings.Contains(message, "required flag") ||
		strings.Contains(message, "invalid argument") {
		return ExitCodeUsage
	}

	return ExitCodeGeneral
}

type PublicError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Status    int    `json:"status,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func ToPublicError(err error) PublicError {
	if err == nil {
		return PublicError{}
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return PublicError{Message: validationErr.Message, Type: TypeValidation}
	}

	var networkErr *NetworkError
	if errors.As(err, &networkErr) {
		return PublicError{Message: networkErr.Message, Type: TypeNetwork}
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		result := PublicError{
			Message:   apiErr.Message,
			Type:      apiErr.Type,
			Status:    apiErr.Status,
			RequestID: apiErr.RequestID,
		}
		if result.Type == "" {
			result.Type = TypeAPI
		}
		return result
	}

	return PublicError{Message: err.Error(), Type: TypeGeneral}
}
