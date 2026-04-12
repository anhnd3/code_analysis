package errors

import "net/http"

type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeInternal        Code = "internal"
	CodeNotImplemented  Code = "not_implemented"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func New(code Code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}

func InvalidArgument(message string) *Error {
	return New(CodeInvalidArgument, message, http.StatusBadRequest)
}

func NotFound(message string) *Error {
	return New(CodeNotFound, message, http.StatusNotFound)
}

func Internal(message string) *Error {
	return New(CodeInternal, message, http.StatusInternalServerError)
}

func NotImplemented(message string) *Error {
	return New(CodeNotImplemented, message, http.StatusNotImplemented)
}

func StatusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if typed, ok := err.(*Error); ok && typed.Status != 0 {
		return typed.Status
	}
	return http.StatusInternalServerError
}
