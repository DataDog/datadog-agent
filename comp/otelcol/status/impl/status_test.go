// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

func TestStatusOut(t *testing.T) {
	tests := []struct {
		name       string
		assertFunc func(t *testing.T, headerProvider status.Provider)
	}{
		{"JSON", func(t *testing.T, headerProvider status.Provider) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T, headerProvider status.Provider) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T, headerProvider status.Provider) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provides := NewComponent(Requires{
				Config: config.NewMock(t),
				Client: ipcmock.New(t).GetClient(),
			})
			headerProvider := provides.StatusProvider.Provider
			test.assertFunc(t, headerProvider)
		})
	}
}

func TestCheckConfigWarnings(t *testing.T) {
	hostmetricsConfig := `
receivers:
  hostmetrics:
    collection_interval: 10s
exporters:
  datadog:
    api:
      key: abc123
`
	namedHostmetricsConfig := `
receivers:
  hostmetrics/production:
    collection_interval: 10s
exporters:
  datadog:
    api:
      key: abc123
`
	noHostmetricsConfig := `
receivers:
  otlp:
    protocols:
      grpc:
exporters:
  datadog:
    api:
      key: abc123
`

	tests := []struct {
		name           string
		runtimeConfig  string
		overrides      map[string]interface{}
		expectWarnings bool
	}{
		{
			name:           "hostmetrics in connected mode",
			runtimeConfig:  hostmetricsConfig,
			overrides:      map[string]interface{}{"otel_standalone": false},
			expectWarnings: true,
		},
		{
			name:           "named hostmetrics instance in connected mode",
			runtimeConfig:  namedHostmetricsConfig,
			overrides:      map[string]interface{}{"otel_standalone": false},
			expectWarnings: true,
		},
		{
			name:           "hostmetrics in standalone mode",
			runtimeConfig:  hostmetricsConfig,
			overrides:      map[string]interface{}{"otel_standalone": true},
			expectWarnings: false,
		},
		{
			name:           "no hostmetrics in connected mode",
			runtimeConfig:  noHostmetricsConfig,
			overrides:      map[string]interface{}{"otel_standalone": false},
			expectWarnings: false,
		},
		{
			name:           "invalid YAML",
			runtimeConfig:  "not: [valid: yaml",
			overrides:      map[string]interface{}{"otel_standalone": false},
			expectWarnings: false,
		},
		{
			name:           "empty runtime config",
			runtimeConfig:  "",
			overrides:      map[string]interface{}{"otel_standalone": false},
			expectWarnings: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, tc.overrides)
			warnings := checkConfigWarnings(tc.runtimeConfig, cfg)
			if tc.expectWarnings {
				require.Len(t, warnings, 1)
				assert.Contains(t, warnings[0], "hostmetrics")
				assert.Contains(t, warnings[0], "connected mode")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestWarningsInTemplateRendering(t *testing.T) {
	warningText := "The hostmetrics receiver is enabled in connected mode. " +
		"The core Datadog Agent already collects host metrics. " +
		"Use standalone mode (DD_OTEL_STANDALONE=true) to avoid metric conflicts."
	statusData := map[string]interface{}{
		"otelAgent": map[string]interface{}{
			"agentVersion":     "7.60.0",
			"collectorVersion": "0.115.0",
			"receiver": map[string]interface{}{
				"spans": 0.0, "metrics": 0.0, "logs": 0.0,
				"refused_spans": 0.0, "refused_metrics": 0.0, "refused_logs": 0.0,
			},
			"exporter": map[string]interface{}{
				"spans": 0.0, "metrics": 0.0, "logs": 0.0,
				"failed_spans": 0.0, "failed_metrics": 0.0, "failed_logs": 0.0,
			},
			"warnings": []string{warningText},
		},
	}

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := status.RenderText(templatesFS, "otelagent.tmpl", b, statusData)
		require.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "Warnings")
		assert.Contains(t, output, "WARNING: "+warningText)
	})

	t.Run("HTML", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := status.RenderHTML(templatesFS, "otelagentHTML.tmpl", b, statusData)
		require.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "Warnings")
		assert.Contains(t, output, warningText)
		assert.Contains(t, output, `class="warning"`)
	})

	t.Run("Text without warnings", func(t *testing.T) {
		noWarningData := map[string]interface{}{
			"otelAgent": map[string]interface{}{
				"agentVersion":     "7.60.0",
				"collectorVersion": "0.115.0",
				"receiver": map[string]interface{}{
					"spans": 0.0, "metrics": 0.0, "logs": 0.0,
					"refused_spans": 0.0, "refused_metrics": 0.0, "refused_logs": 0.0,
				},
				"exporter": map[string]interface{}{
					"spans": 0.0, "metrics": 0.0, "logs": 0.0,
					"failed_spans": 0.0, "failed_metrics": 0.0, "failed_logs": 0.0,
				},
			},
		}
		b := new(bytes.Buffer)
		err := status.RenderText(templatesFS, "otelagent.tmpl", b, noWarningData)
		require.NoError(t, err)
		assert.False(t, strings.Contains(b.String(), "Warnings"))
	})
}
