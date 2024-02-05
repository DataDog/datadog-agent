// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures
var fixturesTemplates embed.FS

func fakeStatusServer(t *testing.T, errCode int, response []byte) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if errCode != 200 {
			http.NotFound(w, r)
		} else {
			_, err := w.Write(response)
			require.NoError(t, err)
		}
	}

	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestStatus(t *testing.T) {
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()

	jsonBytes, err := fixturesTemplates.ReadFile("fixtures/json_response.tmpl")
	assert.NoError(t, err)

	textResponse, err := fixturesTemplates.ReadFile("fixtures/text_response.tmpl")
	assert.NoError(t, err)

	server := fakeStatusServer(t, 200, jsonBytes)
	defer server.Close()

	headerProvider := statusProvider{
		testServerURL: server.URL,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processAgentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["core"])
			assert.Empty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			expected := strings.Replace(string(textResponse), "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)

			assert.Equal(t, expected, output)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusError(t *testing.T) {
	server := fakeStatusServer(t, 500, []byte{})
	defer server.Close()

	headerProvider := statusProvider{
		testServerURL: server.URL,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processAgentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.Equal(t, "\n  Status: Not running or unreachable\n", b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
