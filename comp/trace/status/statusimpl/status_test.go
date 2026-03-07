// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	rarmock "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatusWiring(t *testing.T) {
	// RAR is optional — wiring should succeed with no dependencies provided.
	deps := fxutil.Test[dependencies](t, fx.Options())
	provides := newStatus(deps)
	assert.NotNil(t, provides.StatusProvider.Provider)
}

func TestPopulateStatus(t *testing.T) {
	tests := []struct {
		name   string
		p      statusProvider
		assert func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "valid RAR data",
			p: statusProvider{RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()},
						// expvar values are JSON strings (as returned by expvar.Value.String())
						MainSection: remoteagentregistry.StatusSection{"pid": "12345", "uptime": "60"},
					},
				},
			}},
			assert: func(t *testing.T, r map[string]interface{}) {
				assert.Empty(t, r["error"])
				assert.Equal(t, float64(12345), r["pid"])
			},
		},
		{
			name: "failure reason propagates as error",
			p: statusProvider{RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()},
						FailureReason:   "connection refused",
					},
				},
			}},
			assert: func(t *testing.T, r map[string]interface{}) {
				assert.Equal(t, "connection refused", r["error"])
			},
		},
		{
			name: "non-trace_agent flavor is skipped",
			p: statusProvider{RAR: &rarmock.Component{
				Statuses: []remoteagentregistry.StatusData{
					{RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "system_probe", LastSeen: time.Now()}},
				},
			}},
			assert: func(t *testing.T, r map[string]interface{}) {
				assert.Equal(t, "not running or unreachable", r["error"])
			},
		},
		{
			name: "nil RAR reports not running",
			p:    statusProvider{RAR: nil},
			assert: func(t *testing.T, r map[string]interface{}) {
				assert.Equal(t, "not running or unreachable", r["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, tt.p.populateStatus())
		})
	}
}

func TestJSON(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		p := statusProvider{RAR: &rarmock.Component{
			Statuses: []remoteagentregistry.StatusData{
				{
					RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()},
					MainSection:     remoteagentregistry.StatusSection{"pid": "12345"},
				},
			},
		}}
		stats := make(map[string]interface{})
		err := p.JSON(false, stats)
		assert.NoError(t, err)

		val, ok := stats["apmStats"].(map[string]interface{})
		assert.True(t, ok)
		assert.Empty(t, val["error"])
	})

	t.Run("error case", func(t *testing.T) {
		p := statusProvider{RAR: nil}
		stats := make(map[string]interface{})
		err := p.JSON(false, stats)
		assert.NoError(t, err)

		val, ok := stats["apmStats"].(map[string]interface{})
		assert.True(t, ok)
		assert.NotEmpty(t, val["error"])
	})
}

func TestText(t *testing.T) {
	t.Run("renders Not running when no RAR", func(t *testing.T) {
		p := statusProvider{RAR: nil}
		b := new(bytes.Buffer)
		err := p.Text(false, b)
		assert.NoError(t, err)
		assert.True(t, strings.Contains(b.String(), "Not running or unreachable"))
	})

	t.Run("renders failure reason in error output", func(t *testing.T) {
		p := statusProvider{RAR: &rarmock.Component{
			Statuses: []remoteagentregistry.StatusData{
				{
					RegisteredAgent: remoteagentregistry.RegisteredAgent{Flavor: "trace_agent", LastSeen: time.Now()},
					FailureReason:   "dial tcp: connection refused",
				},
			},
		}}
		b := new(bytes.Buffer)
		err := p.Text(false, b)
		assert.NoError(t, err)
		assert.True(t, strings.Contains(b.String(), "dial tcp"))
	})
}
