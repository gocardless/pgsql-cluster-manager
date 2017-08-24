package errors

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// ErrorWithFields wraps an error message with arbitrary logrus.Fields that can later be
// logged
type ErrorWithFields struct {
	message string
	fields  *logrus.Fields
}

// NewErrorWithFields returns a new ErrorWithFields struct
func NewErrorWithFields(message string, fields *logrus.Fields) ErrorWithFields {
	return ErrorWithFields{message, fields}
}

func (err ErrorWithFields) Error() string {
	return fmt.Sprintf("%s", err.message)
}

// Fields returns the logrus.Fields of this error
func (err ErrorWithFields) Fields() *logrus.Fields {
	return err.fields
}
