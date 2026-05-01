package app

import "net/http"

// Code identifies a category of application error.
type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeInternal        Code = "internal"
	CodeNotImplemented  Code = "not_implemented"
)

// Error is an application-level error with a machine-readable code and HTTP status.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new Error with the given code, message, and HTTP status.
func NewError(code Code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}

// InvalidArgument returns an Error for bad input arguments.
func InvalidArgument(message string) *Error {
	return NewError(CodeInvalidArgument, message, http.StatusBadRequest)
}

// NotFound returns an Error for missing resources.
func NotFound(message string) *Error {
	return NewError(CodeNotFound, message, http.StatusNotFound)
}

// InternalError returns an Error for unexpected internal failures.
func InternalError(message string) *Error {
	return NewError(CodeInternal, message, http.StatusInternalServerError)
}

// NotImplemented returns an Error for features that are deferred.
func NotImplemented(message string) *Error {
	return NewError(CodeNotImplemented, message, http.StatusNotImplemented)
}

// StatusOf extracts the HTTP status from an error value.
func StatusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if typed, ok := err.(*Error); ok && typed.Status != 0 {
		return typed.Status
	}
	return http.StatusInternalServerError
}
