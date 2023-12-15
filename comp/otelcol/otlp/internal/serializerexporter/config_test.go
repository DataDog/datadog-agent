// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestUnmarshalSendCountSum(t *testing.T) {
	newConfig := func(histConfig map[string]interface{}) *confmap.Conf {
		return confmap.NewFromStringMap(map[string]interface{}{
			"metrics": map[string]interface{}{
				"histograms": histConfig,
			},
		})
	}

	tests := []struct {
		name       string
		configMap  *confmap.Conf
		shouldSend bool
		warnings   []string
	}{
		{
			name:      "none set",
			configMap: confmap.New(),
		},
		{
			name: "send_count_sum_metrics=false, send_aggregation_metrics=unset",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics": false,
			}),
			shouldSend: false,
			warnings:   []string{warnDeprecatedSendCountSum},
		},
		{
			name: "send_count_sum_metrics=true, send_aggregation_metrics=unset",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics": true,
			}),
			shouldSend: true,
			warnings:   []string{warnDeprecatedSendCountSum},
		},
		{
			name: "send_count_sum_metrics=unset, send_aggregation_metrics=false",
			configMap: newConfig(map[string]interface{}{
				"send_aggregation_metrics": false,
			}),
			shouldSend: false,
		},
		{
			name: "send_count_sum_metrics=unset, send_aggregation_metrics=true",
			configMap: newConfig(map[string]interface{}{
				"send_aggregation_metrics": true,
			}),
			shouldSend: true,
		},
		{
			name: "send_count_sum_metrics=false, send_aggregation_metrics=false",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics":   false,
				"send_aggregation_metrics": false,
			}),
			shouldSend: false,
			warnings:   []string{warnDeprecatedSendCountSum, warnOverrideSendAggregations},
		},
		{
			name: "send_count_sum_metrics=false, send_aggregation_metrics=true",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics":   false,
				"send_aggregation_metrics": true,
			}),
			shouldSend: false,
			warnings:   []string{warnDeprecatedSendCountSum, warnOverrideSendAggregations},
		},
		{
			name: "send_count_sum_metrics=true, send_aggregation_metrics=false",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics":   true,
				"send_aggregation_metrics": false,
			}),
			shouldSend: true,
			warnings:   []string{warnDeprecatedSendCountSum, warnOverrideSendAggregations},
		},
		{
			name: "send_count_sum_metrics=true, send_aggregation_metrics=true",
			configMap: newConfig(map[string]interface{}{
				"send_count_sum_metrics":   true,
				"send_aggregation_metrics": true,
			}),
			shouldSend: true,
			warnings:   []string{warnDeprecatedSendCountSum, warnOverrideSendAggregations},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg := newDefaultConfig().(*exporterConfig)
			err := cfg.Unmarshal(testInstance.configMap)
			require.NoError(t, err)
			assert.Equal(t, testInstance.shouldSend, cfg.Metrics.HistConfig.SendAggregations)
			assert.ElementsMatch(t, testInstance.warnings, cfg.warnings)
		})
	}
}
