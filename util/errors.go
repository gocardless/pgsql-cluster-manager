package util

import (
	"errors"
)

// ErrorWithFields wraps an error message with arbitrary logrus.Fields that can later be
// logged
type ErrorWithFields struct {
	error
	Fields map[string]interface{}
}

// NewErrorWithFields returns a new ErrorWithFields struct
func NewErrorWithFields(message string, fields map[string]interface{}) ErrorWithFields {
	return ErrorWithFields{errors.New(message), fields}
}
