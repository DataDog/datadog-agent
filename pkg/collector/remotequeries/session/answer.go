// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import "encoding/json"

// Failure codes shared by the integrations. An integration may add codes of its own,
// which its spec lists. Every code is lowercase letters, digits, and underscores, as
// intake requires of an abort reason.
const (
	CodeInvalidRequest         = "invalid_request"
	CodeTargetNotFound         = "target_not_found"
	CodeTargetUnavailable      = "target_unavailable"
	CodeCredentialsUnavailable = "credentials_unavailable"
	CodeConnectionFailed       = "connection_failed"
	CodeQueryFailed            = "query_failed"
	CodeUnsupportedColumn      = "unsupported_column"
	CodeInvalidColumns         = "invalid_columns"
	CodeSchemaTooLarge         = "schema_too_large"
	CodeRowTooLarge            = "row_too_large"
	CodeResultTooLarge         = "result_too_large"
	CodeUploadFailed           = "upload_failed"
	CodeTimeout                = "timeout"
	CodeCancelled              = "cancelled"
	CodeInternalError          = "internal_error"
)

// Failure is a run's final error: a stable code and a message that never contains the
// query text, result values, credentials, connection strings, or driver error text.
type Failure struct {
	Code    string
	Message string
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

// Fail returns a Failure.
func Fail(code, message string) *Failure {
	return &Failure{Code: code, Message: message}
}

type answer struct {
	Status string        `json:"status"`
	Error  *answerDetail `json:"error,omitempty"`
}

type answerDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func encodeAnswer(a answer) string {
	out, err := json.Marshal(a)
	if err != nil {
		return `{"status":"FAILED","error":{"code":"internal_error","message":"the answer could not be encoded"}}`
	}
	return string(out)
}

// Matched is the answer of a check that serves the resolved target.
func Matched() string { return encodeAnswer(answer{Status: "MATCHED"}) }

// Succeeded is the answer of a run whose upload intake finalized.
func Succeeded() string { return encodeAnswer(answer{Status: "SUCCEEDED"}) }

// Failed is the answer of a call that failed.
func Failed(f *Failure) string {
	return encodeAnswer(answer{Status: "FAILED", Error: &answerDetail{Code: f.Code, Message: f.Message}})
}
