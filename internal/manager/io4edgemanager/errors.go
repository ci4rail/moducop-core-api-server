package io4edgemanager

import (
	"errors"
	"fmt"
)

const (
	ErrCodeDeviceUpdateInProgress  = "io4e-0001"
	ErrCodeAlreadyDeployed         = "io4e-0002"
	ErrCodeArtifactInvalid         = "io4e-0003"
	ErrCodeDeviceNotFound          = "io4e-0004"
)

type CodedError interface {
	error
	Code() string
	Message() string
}

type codedError struct {
	code    string
	message string
	cause   error
}

func (e *codedError) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("%s: %s", e.code, e.message)
	}
	return fmt.Sprintf("%s: %s: %v", e.code, e.message, e.cause)
}

func (e *codedError) Unwrap() error { return e.cause }
func (e *codedError) Code() string  { return e.code }
func (e *codedError) Message() string {
	return e.message
}

func NewCodedError(code, message string) error {
	return &codedError{code: code, message: message}
}

func WrapCodedError(code, message string, cause error) error {
	return &codedError{code: code, message: message, cause: cause}
}

func ExtractCode(err error) (code string, message string, ok bool) {
	var ce CodedError
	if !errors.As(err, &ce) {
		return "", "", false
	}
	return ce.Code(), ce.Message(), true
}
