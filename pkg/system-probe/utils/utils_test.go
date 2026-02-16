// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTruthy(t *testing.T) {
	for _, v := range []string{"1", "t", "true", "True", "TRUE", "yes", "y", "on", " true "} {
		assert.True(t, isTruthy(v), "expected %q to be truthy", v)
	}
	for _, v := range []string{"0", "false", "no", "off", "", "random"} {
		assert.False(t, isTruthy(v), "expected %q to be falsy", v)
	}
}

func TestGetPrettyPrintFromQueryParams(t *testing.T) {
	req := httptest.NewRequest("GET", "/?pretty_print=true", nil)
	assert.Equal(t, FormatOptions(PrettyPrint), GetPrettyPrintFromQueryParams(req))

	req = httptest.NewRequest("GET", "/", nil)
	assert.Equal(t, FormatOptions(CompactOutput), GetPrettyPrintFromQueryParams(req))

	req = httptest.NewRequest("GET", "/?pretty_print=false", nil)
	assert.Equal(t, FormatOptions(CompactOutput), GetPrettyPrintFromQueryParams(req))
}

func TestWriteAsJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteAsJSON(w, map[string]string{"key": "value"}, CompactOutput)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"key":"value"`)

	// pretty print adds indentation
	w = httptest.NewRecorder()
	WriteAsJSON(w, map[string]string{"key": "value"}, PrettyPrint)
	assert.Contains(t, w.Body.String(), "  ")
}

func TestGetClientID(t *testing.T) {
	req := httptest.NewRequest("GET", "/?client_id=42", nil)
	assert.Equal(t, "42", GetClientID(req))

	req = httptest.NewRequest("GET", "/", nil)
	assert.Equal(t, "-1", GetClientID(req))
}
