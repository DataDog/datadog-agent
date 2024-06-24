// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package utils has common utility methods that components can use for structuring http responses of their endpoints
package utils

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	err := errors.New("some error")
	errorCode := http.StatusInternalServerError

	SetJSONError(w, err, errorCode)

	res := w.Result()
	defer res.Body.Close()

	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	// Verify the response body
	expectedBody := "{\"error\":\"some error\"}\n"

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.EqualValues(t, []byte(expectedBody), body)

	// Verify the response status code
	assert.Equal(t, errorCode, res.StatusCode)
}
