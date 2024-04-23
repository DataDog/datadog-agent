// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"bytes"
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
	jsonBytes, err := fixturesTemplates.ReadFile("fixtures/expvar_response.tmpl")
	assert.NoError(t, err)

	server := fakeStatusServer(t, 200, jsonBytes)
	defer server.Close()

	configComponent := fxutil.Test[config.Component](t, config.MockModule())

	headerProvider := StatusProvider{
		testServerURL: server.URL,
		config:        configComponent,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processComponentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)
			assert.NotEmpty(t, val["core"])
			assert.Empty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.True(t, strings.Contains(b.String(), "API Key ending with:"))
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

	errorResponse, err := fixturesTemplates.ReadFile("fixtures/text_error_response.tmpl")
	assert.NoError(t, err)

	configComponent := fxutil.Test[config.Component](t, config.MockModule())

	headerProvider := StatusProvider{
		testServerURL: server.URL,
		config:        configComponent,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processComponentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			expected := strings.Replace(string(errorResponse), "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)

			assert.Equal(t, expected, output)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
