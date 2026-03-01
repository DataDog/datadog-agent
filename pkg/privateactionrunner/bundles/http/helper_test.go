// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- validateHeaders ---

// TestValidateHeaders_AcceptsNormalHeaders verifies that common, non-restricted headers pass.
func TestValidateHeaders_AcceptsNormalHeaders(t *testing.T) {
	headers := []Header{
		{Key: "Authorization", Value: []string{"Bearer token"}},
		{Key: "Content-Type", Value: []string{"application/json"}},
		{Key: "X-Custom-Header", Value: []string{"value"}},
	}
	assert.NoError(t, validateHeaders(headers))
}

// TestValidateHeaders_RejectsForbiddenHeaders verifies that each restricted header name
// causes an error regardless of how it is capitalised.
func TestValidateHeaders_RejectsForbiddenHeaders(t *testing.T) {
	forbidden := []string{
		"Content-Length",
		"Host",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"Forwarded",
		"Sec-Datadog",
	}
	for _, name := range forbidden {
		t.Run(name, func(t *testing.T) {
			err := validateHeaders([]Header{{Key: name, Value: []string{"x"}}})
			require.Error(t, err, "header %q must be rejected", name)
			assert.Contains(t, err.Error(), "unsupported header")
		})
	}
}

// TestValidateHeaders_ForbiddenHeaderCaseInsensitive verifies that the check is
// case-insensitive (e.g., "content-length" is as forbidden as "Content-Length").
func TestValidateHeaders_ForbiddenHeaderCaseInsensitive(t *testing.T) {
	err := validateHeaders([]Header{{Key: "CONTENT-LENGTH", Value: []string{"100"}}})
	require.Error(t, err)
}

// TestValidateHeaders_EmptyInputSucceeds verifies that no headers means no error.
func TestValidateHeaders_EmptyInputSucceeds(t *testing.T) {
	assert.NoError(t, validateHeaders(nil))
	assert.NoError(t, validateHeaders([]Header{}))
}

// --- shouldThrowForHTTPErrorStatus ---

// TestShouldThrowForHTTPErrorStatus_StatusInRange verifies that a status code falling within
// a specified range causes an error to be returned.
func TestShouldThrowForHTTPErrorStatus_StatusInRange(t *testing.T) {
	err := shouldThrowForHTTPErrorStatus(404, []string{"400-499"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// TestShouldThrowForHTTPErrorStatus_StatusOutsideRange verifies that a code outside the range
// returns nil (not an error).
func TestShouldThrowForHTTPErrorStatus_StatusOutsideRange(t *testing.T) {
	err := shouldThrowForHTTPErrorStatus(200, []string{"400-499"})
	assert.NoError(t, err)
}

// TestShouldThrowForHTTPErrorStatus_ExactCode verifies that a single exact status code
// also triggers an error when matched.
func TestShouldThrowForHTTPErrorStatus_ExactCode(t *testing.T) {
	err := shouldThrowForHTTPErrorStatus(500, []string{"500"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestShouldThrowForHTTPErrorStatus_RangeAndExact verifies mixed entries: a range and a
// specific code in the same slice.
func TestShouldThrowForHTTPErrorStatus_RangeAndExact(t *testing.T) {
	ranges := []string{"400-499", "503"}

	assert.Error(t, shouldThrowForHTTPErrorStatus(401, ranges))
	assert.Error(t, shouldThrowForHTTPErrorStatus(503, ranges))
	assert.NoError(t, shouldThrowForHTTPErrorStatus(200, ranges))
	assert.NoError(t, shouldThrowForHTTPErrorStatus(500, ranges))
}

// TestShouldThrowForHTTPErrorStatus_RangeBoundaryEdgeCases verifies that the range is
// inclusive at both ends.
func TestShouldThrowForHTTPErrorStatus_RangeBoundaryInclusive(t *testing.T) {
	assert.Error(t, shouldThrowForHTTPErrorStatus(400, []string{"400-499"}), "lower bound must be included")
	assert.Error(t, shouldThrowForHTTPErrorStatus(499, []string{"400-499"}), "upper bound must be included")
	assert.NoError(t, shouldThrowForHTTPErrorStatus(399, []string{"400-499"}))
	assert.NoError(t, shouldThrowForHTTPErrorStatus(500, []string{"400-499"}))
}

// TestShouldThrowForHTTPErrorStatus_InvalidRangeFormat verifies that a malformed range
// string (wrong separator count, non-numeric parts, inverted bounds) returns a parse error.
func TestShouldThrowForHTTPErrorStatus_InvalidRangeFormat(t *testing.T) {
	cases := []string{
		"400-499-599",    // three parts
		"abc-499",        // non-numeric start
		"400-xyz",        // non-numeric end
		"invalid",        // non-numeric single code
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			err := shouldThrowForHTTPErrorStatus(400, []string{c})
			require.Error(t, err, "malformed range %q should return a parse error", c)
		})
	}
}

// TestShouldThrowForHTTPErrorStatus_EmptyRanges verifies that an empty slice means
// no status code is an error.
func TestShouldThrowForHTTPErrorStatus_EmptyRanges(t *testing.T) {
	assert.NoError(t, shouldThrowForHTTPErrorStatus(500, []string{}))
	assert.NoError(t, shouldThrowForHTTPErrorStatus(500, nil))
}

// --- hasSameDomain ---

// TestHasSameDomain_MatchingHosts verifies that two URLs with identical hosts match.
func TestHasSameDomain_MatchingHosts(t *testing.T) {
	assert.True(t, hasSameDomain("https://api.example.com/v1/endpoint", "https://api.example.com/v2/other"))
}

// TestHasSameDomain_DifferentHosts verifies that URLs with different hostnames do not match.
func TestHasSameDomain_DifferentHosts(t *testing.T) {
	assert.False(t, hasSameDomain("https://api.example.com/path", "https://other.example.com/path"))
}

// TestHasSameDomain_InvalidURL verifies that an unparseable source URL causes false to be
// returned rather than panicking.
func TestHasSameDomain_InvalidURL(t *testing.T) {
	assert.False(t, hasSameDomain("://\x00bad", "https://api.example.com"))
	assert.False(t, hasSameDomain("https://api.example.com", "://\x00bad"))
}

// --- httpErrResponseToResultErr ---

// TestHttpErrResponseToResultErr_EmptyBody verifies that the error message is just the
// HTTP status string when no body is present.
func TestHttpErrResponseToResultErr_EmptyBody(t *testing.T) {
	resp := &http.Response{Status: "404 Not Found"}
	err := httpErrResponseToResultErr(resp, "")
	require.Error(t, err)
	assert.Equal(t, "404 Not Found", err.Error())
}

// TestHttpErrResponseToResultErr_ShortBody verifies that a short body is appended in full.
func TestHttpErrResponseToResultErr_ShortBody(t *testing.T) {
	resp := &http.Response{Status: "400 Bad Request"}
	err := httpErrResponseToResultErr(resp, `{"error":"invalid input"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400 Bad Request")
	assert.Contains(t, err.Error(), "invalid input")
}

// TestHttpErrResponseToResultErr_LongBodyTruncated verifies that a body exceeding
// MaxResponseErrMsgLen characters is truncated with a "[truncated]" suffix. This is
// important to prevent excessively large error messages from flooding logs.
func TestHttpErrResponseToResultErr_LongBodyTruncated(t *testing.T) {
	resp := &http.Response{Status: "500 Internal Server Error"}
	longBody := strings.Repeat("x", MaxResponseErrMsgLen+100)

	err := httpErrResponseToResultErr(resp, longBody)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "[truncated]", "long body must be truncated")
	assert.NotContains(t, err.Error(), longBody, "full long body must not appear in the error")
}

// TestHttpErrResponseToResultErr_ExactlyMaxLen verifies that a body at exactly
// MaxResponseErrMsgLen characters is NOT truncated (boundary condition).
func TestHttpErrResponseToResultErr_ExactlyMaxLen(t *testing.T) {
	resp := &http.Response{Status: "422 Unprocessable Entity"}
	body := strings.Repeat("a", MaxResponseErrMsgLen)

	err := httpErrResponseToResultErr(resp, body)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "[truncated]", "body at exact max length must not be truncated")
}

// --- consolidateBody ---

// TestConsolidateBody_JSONMergesMaps verifies that when the content type is JSON, the
// credential body object is merged into the input body map, with credential keys taking
// precedence over input keys (connection overrides action).
func TestConsolidateBody_JSONMergesCredentialIntoInput(t *testing.T) {
	inputBody := map[string]interface{}{"action_key": "action_val", "shared": "from_action"}
	credentialBody := `{"cred_key": "cred_val", "shared": "from_creds"}`

	result, err := consolidateBody("application/json", inputBody, credentialBody)

	require.NoError(t, err)
	merged, ok := result.(map[string]interface{})
	require.True(t, ok, "result must be a map")
	assert.Equal(t, "action_val", merged["action_key"])
	assert.Equal(t, "cred_val", merged["cred_key"])
	assert.Equal(t, "from_creds", merged["shared"], "connection values override action values")
}

// TestConsolidateBody_JSONNilInputReturnsCredentialBody verifies that when there is no
// action input body, the credential body is returned as-is.
func TestConsolidateBody_JSONNilInputReturnsCredentialBody(t *testing.T) {
	result, err := consolidateBody("application/json", nil, `{"key":"value"}`)

	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, result)
}

// TestConsolidateBody_PlainTextConcatenates verifies that non-JSON content types concatenate
// the credential body and action body with a CRLF separator.
func TestConsolidateBody_PlainTextConcatenates(t *testing.T) {
	result, err := consolidateBody("text/plain", "action body", "credential body")

	require.NoError(t, err)
	resultStr, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, "credential body\r\naction body", resultStr)
}

// TestConsolidateBody_PlainTextNilInputReturnsCredentialOnly verifies that with no action
// body, the credential body alone is returned.
func TestConsolidateBody_PlainTextNilInputReturnsCredentialOnly(t *testing.T) {
	result, err := consolidateBody("text/plain", nil, "credential body")

	require.NoError(t, err)
	assert.Equal(t, "credential body", result)
}
