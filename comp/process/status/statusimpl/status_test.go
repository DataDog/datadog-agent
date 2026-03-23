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

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	rarmock "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/mock"
)

//go:embed fixtures
var fixturesTemplates embed.FS

// makeProcessAgentRAR builds a mock RAR that returns a pre-built status JSON
// (as the process agent's GetStatusDetails now produces).
func makeProcessAgentRAR(t *testing.T) *rarmock.Component {
	t.Helper()

	// Read the fixture which represents the full pre-built Status struct
	// that the process agent would return via RAR.
	statusBytes, err := fixturesTemplates.ReadFile("fixtures/process_status.json")
	require.NoError(t, err)

	return &rarmock.Component{
		Statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{
					Flavor:      "process_agent",
					DisplayName: "Process Agent",
					LastSeen:    time.Now(),
				},
				MainSection: remoteagentregistry.StatusSection{
					"status": string(statusBytes),
				},
			},
		},
	}
}

func TestStatus(t *testing.T) {
	headerProvider := statusProvider{
		rar: makeProcessAgentRAR(t),
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

	rar := &rarmock.Component{
		Statuses: []remoteagentregistry.StatusData{
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
		rar: rar,
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

func TestPopulateStatusNilRAR(t *testing.T) {
	p := statusProvider{rar: nil}
	result := p.populateStatus()
	assert.Equal(t, "not running or unreachable", result["error"])
}

func TestPopulateStatusWrongFlavor(t *testing.T) {
	p := statusProvider{rar: &rarmock.Component{
		Statuses: []remoteagentregistry.StatusData{
			{RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()}},
		},
	}}
	result := p.populateStatus()
	assert.Equal(t, "not running or unreachable", result["error"])
}

func TestPopulateStatusValidJSON(t *testing.T) {
	status := map[string]interface{}{
		"date": 1234567890.0,
		"core": map[string]interface{}{"version": "7.78.0"},
	}
	statusBytes, _ := json.Marshal(status)

	p := statusProvider{rar: &rarmock.Component{
		Statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "process_agent", LastSeen: time.Now()},
				MainSection:     remoteagentregistry.StatusSection{"status": string(statusBytes)},
			},
		},
	}}
	result := p.populateStatus()
	assert.NotEmpty(t, result["core"])
	assert.Empty(t, result["error"])
}
