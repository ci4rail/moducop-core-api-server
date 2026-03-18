/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"errors"
	"fmt"
)

const (
	ErrCodeInvalidCoreOSEntityName = "cpm-0001"
	ErrCodeEntityUpdateInProgress  = "cpm-0002"
	ErrCodeMenderBusy              = "cpm-0003"
	ErrCodeStartUpdateFailed       = "cpm-0004"
	ErrCodeAlreadyDeployed         = "cpm-0005"
	ErrCodeArtifactInvalid         = "cpm-0006"
	ErrCodeGetVersionFailed        = "cpm-0007"
	ErrCodeListApplicationsFailed  = "cpm-0008"
	ErrCodeEntityNotFound          = "cpm-0009"
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
