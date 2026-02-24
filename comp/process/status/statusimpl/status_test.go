// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

//go:embed fixtures
var fixturesTemplates embed.FS

// mockRAR is a minimal implementation of remoteagentregistry.Component for testing.
type mockRAR struct {
	statuses []remoteagentregistry.StatusData
}

func (m *mockRAR) RegisterRemoteAgent(_ *remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "", 0, nil
}
func (m *mockRAR) RefreshRemoteAgent(_ string) bool                           { return false }
func (m *mockRAR) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent { return nil }
func (m *mockRAR) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	return m.statuses
}

func makeProcessAgentRAR(t *testing.T) *mockRAR {
	jsonBytes, err := fixturesTemplates.ReadFile("fixtures/expvar_response.tmpl")
	require.NoError(t, err)

	var full map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(jsonBytes, &full))

	return &mockRAR{
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{
					Flavor:      "process_agent",
					DisplayName: "Process Agent",
					LastSeen:    time.Now(),
				},
				MainSection: remoteagentregistry.StatusSection{
					"process_agent": string(full["process_agent"]),
				},
			},
		},
	}
}

func TestStatus(t *testing.T) {
	configComponent := config.NewMock(t)

	headerProvider := statusProvider{
		config:   configComponent,
		hostname: hostnameimpl.NewHostnameService(),
		rar:      makeProcessAgentRAR(t),
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
	errorResponse, err := fixturesTemplates.ReadFile("fixtures/text_error_response.tmpl")
	require.NoError(t, err)

	configComponent := config.NewMock(t)

	// RAR reports the process agent as failed.
	rar := &mockRAR{
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{
					Flavor:   "process_agent",
					LastSeen: time.Now(),
				},
				FailureReason: "connection refused",
			},
		},
	}

	headerProvider := statusProvider{
		config: configComponent,
		rar:    rar,
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
