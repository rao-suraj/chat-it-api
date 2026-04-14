package errors

import "net/http"

type AppError struct {
	Status  int
	Code    string
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func BadRequest(code, message string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: code, Message: message}
}

func NotFound(message string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: message}
}

func Internal() *AppError {
	return &AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "something went wrong"}
}