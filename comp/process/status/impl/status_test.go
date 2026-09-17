// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		_, err := w.Write(jsonBytes)
		require.NoError(t, err)
	}))
	defer server.Close()

	configComponent := config.NewMock(t)
	configComponent.SetInTest("cloud_provider_metadata", []string{})
	provider := statusProvider{
		testServerURL: server.URL,
		config:        configComponent,
		hostname:      hostnameimpl.NewHostnameService(),
	}

	response, err := provider.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")
	assert.EqualValues(t, 1, requestCount.Load(), "rendered and JSON status must share one snapshot")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.JsonPayload, &payload))
	processAgentStatus, ok := payload["processAgentStatus"].(map[string]interface{})
	require.True(t, ok)
	var expectedExpvars map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &expectedExpvars))
	require.Equal(t, expectedExpvars, processAgentStatus["expvars"])
	var expected bytes.Buffer
	require.NoError(t, provider.renderTextFromStatus(payload, &expected))
	assert.Equal(t, expected.String(), response.NamedSections["Details"].Fields[""])
}

func TestGetStatusDetailsCancellation(t *testing.T) {
	const cancellationTimeout = 5 * time.Second

	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-request.Context().Done():
			close(requestCanceled)
		case <-releaseRequest:
		}
	}))
	defer func() {
		close(releaseRequest)
		server.Close()
	}()

	configComponent := config.NewMock(t)
	configComponent.SetInTest("cloud_provider_metadata", []string{})
	provider := statusProvider{
		testServerURL: server.URL,
		config:        configComponent,
		hostname:      hostnameimpl.NewHostnameService(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := provider.GetStatusDetails(ctx, &pbcore.GetStatusDetailsRequest{})
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(cancellationTimeout):
		t.Fatal("expvar request did not start")
	}
	cancel()

	select {
	case <-requestCanceled:
	case <-time.After(cancellationTimeout):
		t.Fatal("expvar request was not canceled")
	}

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(cancellationTimeout):
		t.Fatal("GetStatusDetails did not return after cancellation")
	}
}
