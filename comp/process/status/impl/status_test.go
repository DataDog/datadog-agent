// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"context"
	"embed"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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

	configComponent := config.NewMock(t)
	configComponent.SetInTest("cloud_provider_metadata", []string{})

	headerProvider := statusProvider{
		testServerURL: server.URL,
		config:        configComponent,
		hostname:      hostnameimpl.NewHostnameService(),
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

	configComponent := config.NewMock(t)

	headerProvider := statusProvider{
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
			processStats := stats["processAgentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			expected := strings.ReplaceAll(string(errorResponse), "\r\n", "\n")
			output := strings.ReplaceAll(b.String(), "\r\n", "\n")

			assert.Equal(t, expected, output)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestGetStatusDetails(t *testing.T) {
	jsonBytes, err := fixturesTemplates.ReadFile("fixtures/expvar_response.tmpl")
	require.NoError(t, err)
	statusDatePattern := regexp.MustCompile(`(?m)^  Status date: .*$`)

	tests := []struct {
		name       string
		statusCode int
		response   []byte
	}{
		{
			name:       "successful status",
			statusCode: http.StatusOK,
			response:   jsonBytes,
		},
		{
			name:       "unreachable status",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := fakeStatusServer(t, test.statusCode, test.response)
			defer server.Close()

			configComponent := config.NewMock(t)
			configComponent.SetInTest("cloud_provider_metadata", []string{})

			provider := statusProvider{
				testServerURL: server.URL,
				config:        configComponent,
				hostname:      hostnameimpl.NewHostnameService(),
			}

			var expected bytes.Buffer
			require.NoError(t, provider.Text(false, &expected))

			response, err := provider.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
			require.NoError(t, err)
			require.Contains(t, response.NamedSections, "Details")
			expectedDetails := statusDatePattern.ReplaceAllString(expected.String(), "  Status date: <dynamic>")
			actualDetails := statusDatePattern.ReplaceAllString(response.NamedSections["Details"].Fields[""], "  Status date: <dynamic>")
			assert.Equal(t, expectedDetails, actualDetails)
		})
	}
}
