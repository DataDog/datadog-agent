// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tagenrichmentprocessor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/processor/processortest"

        "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/tagenrichmentprocessor/internal"
)

func TestType(t *testing.T) {
	factory := NewFactory()
	pType := factory.Type()

	assert.Equal(t, pType, metadata.Type)
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))
}

func TestCreateProcessors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configName string
		succeed    bool
	}{
		{
			configName: "config_logs_strict.yaml",
			succeed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.configName, func(t *testing.T) {
			cm, err := confmaptest.LoadConf(filepath.Join("testdata", tt.configName))
			require.NoError(t, err)

			for k := range cm.ToStringMap() {
				// Check if all processor variations that are defined in test config can be actually created
				factory := NewFactory()
				cfg := factory.CreateDefaultConfig()

				sub, err := cm.Sub(k)
				require.NoError(t, err)
				require.NoError(t, component.UnmarshalConfig(sub, cfg))

				tp, tErr := factory.CreateTracesProcessor(
					context.Background(),
					processortest.NewNopCreateSettings(),
					cfg, consumertest.NewNop(),
				)
				mp, mErr := factory.CreateMetricsProcessor(
					context.Background(),
					processortest.NewNopCreateSettings(),
					cfg,
					consumertest.NewNop(),
				)
				if strings.Contains(tt.configName, "traces") {
					assert.Equal(t, tt.succeed, tp != nil)
					assert.Equal(t, tt.succeed, tErr == nil)

					assert.NotNil(t, mp)
					assert.Nil(t, mErr)
				} else {
					// Should not break configs with no trace data
					assert.NotNil(t, tp)
					assert.Nil(t, tErr)

					assert.Equal(t, tt.succeed, mp != nil)
					assert.Equal(t, tt.succeed, mErr == nil)
				}
			}
		})
	}
}
