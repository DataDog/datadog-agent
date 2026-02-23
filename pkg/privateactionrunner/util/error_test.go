// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"errors"
	"testing"

	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPARError_SetsInternalAndExternalMessageToSameValue verifies that when a single error
// is provided, both Message and ExternalMessage are set to the same string. This matters because
// the Message is what gets written to internal logs while ExternalMessage surfaces to callers.
func TestNewPARError_SetsInternalAndExternalMessageToSameValue(t *testing.T) {
	err := NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("something broke"))

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, err.ErrorCode)
	assert.Equal(t, "something broke", err.Message)
	assert.Equal(t, "something broke", err.ExternalMessage)
	assert.False(t, err.Retryable)
}

// TestNewPARErrorWithDisplayError_SeparatesInternalAndExternalMessages verifies that the
// internal log message (Message) and the user-facing message (ExternalMessage) can differ.
func TestNewPARErrorWithDisplayError_SeparatesInternalAndExternalMessages(t *testing.T) {
	err := NewPARErrorWithDisplayError(
		aperrorpb.ActionPlatformErrorCode_CONNECTION_ERROR,
		errors.New("tcp connect: connection refused on 10.0.0.1:443"),
		"Failed to connect to the target host",
	)

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_CONNECTION_ERROR, err.ErrorCode)
	assert.Equal(t, "tcp connect: connection refused on 10.0.0.1:443", err.Message)
	assert.Equal(t, "Failed to connect to the target host", err.ExternalMessage)
}

// TestPARError_ErrorMethod_ContainsMessageAndExternalMessage verifies the string representation
// that appears in logs includes both fields so they are easily searchable.
func TestPARError_ErrorMethod_ContainsMessageAndExternalMessage(t *testing.T) {
	err := NewPARErrorWithDisplayError(
		aperrorpb.ActionPlatformErrorCode_EXPIRED_TASK,
		errors.New("internal: task ttl exceeded"),
		"task has expired",
	)

	s := err.Error()

	assert.Contains(t, s, "internal: task ttl exceeded")
	assert.Contains(t, s, "task has expired")
}

// TestDefaultPARError_PreservesExistingPARErrorCode verifies that wrapping a PARError through
// DefaultPARError does not replace its error code. This ensures that specific codes (e.g.
// SIGNATURE_ERROR, MISMATCHED_ORG_ID) survive being passed through generic error handlers
// and ultimately appear correctly in logs and metrics.
func TestDefaultPARError_PreservesExistingPARErrorCode(t *testing.T) {
	original := NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, errors.New("bad sig"))

	result := DefaultPARError(original)

	require.Equal(t, aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, result.ErrorCode,
		"DefaultPARError must not replace a PARError's error code with INTERNAL_ERROR")
	assert.Equal(t, original.Message, result.Message)
}

// TestDefaultPARError_WrapsPlainErrorWithInternalErrorCode verifies that a generic Go error
// is assigned the INTERNAL_ERROR code so that unhandled errors have a deterministic log entry.
func TestDefaultPARError_WrapsPlainErrorWithInternalErrorCode(t *testing.T) {
	plain := errors.New("unexpected nil pointer")

	result := DefaultPARError(plain)

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, result.ErrorCode)
	assert.Equal(t, plain.Error(), result.Message)
}

// TestDefaultActionError_PreservesExistingPARErrorCode verifies that a PARError passed to
// DefaultActionError retains its original code (e.g. CONNECTION_ERROR) rather than being
// overwritten with ACTION_ERROR.
func TestDefaultActionError_PreservesExistingPARErrorCode(t *testing.T) {
	original := NewPARError(aperrorpb.ActionPlatformErrorCode_CONNECTION_ERROR, errors.New("conn failed"))

	result := DefaultActionError(original)

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_CONNECTION_ERROR, result.ErrorCode)
}

// TestDefaultActionError_WrapsPlainErrorWithActionErrorCode verifies that action-level errors
// from bundles receive the ACTION_ERROR code, distinguishing them from INTERNAL_ERROR in logs.
func TestDefaultActionError_WrapsPlainErrorWithActionErrorCode(t *testing.T) {
	plain := errors.New("target returned 404")

	result := DefaultActionError(plain)

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_ACTION_ERROR, result.ErrorCode)
	assert.Equal(t, plain.Error(), result.Message)
}

// TestDefaultActionErrorWithDisplayError_SeparatesMessages verifies the display error variant
// of the action error wrapping.
func TestDefaultActionErrorWithDisplayError_SeparatesMessages(t *testing.T) {
	result := DefaultActionErrorWithDisplayError(
		errors.New("GET /api/v1/pods: 403 Forbidden"),
		"Kubernetes API returned permission denied",
	)

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_ACTION_ERROR, result.ErrorCode)
	assert.Equal(t, "GET /api/v1/pods: 403 Forbidden", result.Message)
	assert.Equal(t, "Kubernetes API returned permission denied", result.ExternalMessage)
}

// TestDefaultActionErrorWithDisplayError_PreservesExistingPARError verifies that an existing
// PARError is not overwritten even when a custom display error string is provided.
func TestDefaultActionErrorWithDisplayError_PreservesExistingPARError(t *testing.T) {
	original := NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, errors.New("key missing"))

	result := DefaultActionErrorWithDisplayError(original, "some display msg")

	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, result.ErrorCode,
		"pre-existing PARError must survive DefaultActionErrorWithDisplayError")
}
