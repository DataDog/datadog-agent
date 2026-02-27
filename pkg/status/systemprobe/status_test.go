// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systemprobe

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	rarmock "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/mock"
)

func makeModuleStatsJSON(t *testing.T) string {
	t.Helper()
	stats := map[string]interface{}{
		"uptime":     "1h",
		"updated_at": float64(time.Now().Unix()),
	}
	b, err := json.Marshal(stats)
	require.NoError(t, err)
	return string(b)
}

func TestPopulateStatus(t *testing.T) {
	tests := []struct {
		name        string
		provider    Provider
		assertStats func(t *testing.T, stats map[string]interface{})
	}{
		{
			name: "valid RAR data sets module stats",
			provider: Provider{
				RAR: &rarmock.Component{
					Statuses: []remoteagentregistry.StatusData{
						{
							RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()},
							MainSection:     remoteagentregistry.StatusSection{"modules": makeModuleStatsJSON(t)},
						},
					},
				},
			},
			assertStats: func(t *testing.T, stats map[string]interface{}) {
				val, ok := stats["systemProbeStats"].(map[string]interface{})
				assert.True(t, ok)
				assert.Empty(t, val["Errors"])
				assert.NotEmpty(t, val["uptime"])
			},
		},
		{
			name: "failure reason propagates as error",
			provider: Provider{
				RAR: &rarmock.Component{
					Statuses: []remoteagentregistry.StatusData{
						{
							RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()},
							FailureReason:   "connection refused",
						},
					},
				},
			},
			assertStats: func(t *testing.T, stats map[string]interface{}) {
				val, ok := stats["systemProbeStats"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "connection refused", val["Errors"])
			},
		},
		{
			name: "non-system_probe flavors are skipped",
			provider: Provider{
				RAR: &rarmock.Component{
					Statuses: []remoteagentregistry.StatusData{
						{RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()}},
					},
				},
			},
			assertStats: func(t *testing.T, stats map[string]interface{}) {
				val, ok := stats["systemProbeStats"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "not running or unreachable", val["Errors"])
			},
		},
		{
			name:     "nil RAR reports not running",
			provider: Provider{RAR: nil},
			assertStats: func(t *testing.T, stats map[string]interface{}) {
				val, ok := stats["systemProbeStats"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "not running or unreachable", val["Errors"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := make(map[string]interface{})
			tt.provider.populateStatus(stats)
			tt.assertStats(t, stats)
		})
	}
}

func TestJSON(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		p := Provider{
			RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()},
						MainSection:     remoteagentregistry.StatusSection{"modules": makeModuleStatsJSON(t)},
					},
				},
			},
		}
		stats := make(map[string]interface{})
		err := p.JSON(false, stats)
		assert.NoError(t, err)

		val, ok := stats["systemProbeStats"].(map[string]interface{})
		assert.True(t, ok)
		assert.Empty(t, val["Errors"])
	})

	t.Run("error case", func(t *testing.T) {
		p := Provider{RAR: nil}
		stats := make(map[string]interface{})
		err := p.JSON(false, stats)
		assert.NoError(t, err)

		val, ok := stats["systemProbeStats"].(map[string]interface{})
		assert.True(t, ok)
		assert.NotEmpty(t, val["Errors"])
	})
}

func TestText(t *testing.T) {
	t.Run("renders Running when data is present", func(t *testing.T) {
		p := Provider{
			RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()},
						MainSection:     remoteagentregistry.StatusSection{"modules": makeModuleStatsJSON(t)},
					},
				},
			},
		}
		b := new(bytes.Buffer)
		err := p.Text(false, b)
		assert.NoError(t, err)
		assert.True(t, strings.Contains(b.String(), "Running"))
	})

	t.Run("renders Not running on error", func(t *testing.T) {
		p := Provider{RAR: nil}
		b := new(bytes.Buffer)
		err := p.Text(false, b)
		assert.NoError(t, err)
		assert.True(t, strings.Contains(b.String(), "Not running or unreachable"))
	})

	t.Run("renders failure reason in error output", func(t *testing.T) {
		p := Provider{
			RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()},
						FailureReason:   "dial unix /var/run/sysprobe.sock: no such file",
					},
				},
			},
		}
		b := new(bytes.Buffer)
		err := p.Text(false, b)
		assert.NoError(t, err)
		assert.True(t, strings.Contains(b.String(), "dial unix"))
	})
}
