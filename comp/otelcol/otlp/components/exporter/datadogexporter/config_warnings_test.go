// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
)

func TestSendAggregations(t *testing.T) {
	tests := []struct {
		name              string
		cfgMap            *confmap.Conf
		expectedAggrValue bool
		warnings          []string
		err               string
	}{
		{
			name: "both metrics::histograms::send_count_sum_metrics and metrics::histograms::send_aggregation_metrics",
			cfgMap: confmap.NewFromStringMap(map[string]any{
				"metrics": map[string]any{
					"histograms": map[string]any{
						"send_count_sum_metrics":   true,
						"send_aggregation_metrics": true,
					},
				},
			}),
			err: "\"metrics::histograms::send_count_sum_metrics\" and \"metrics::histograms::send_aggregation_metrics\" can't be both set at the same time: use \"metrics::histograms::send_aggregation_metrics\" only instead",
		},
		{
			name: "metrics::histograms::send_count_sum_metrics set to true",
			cfgMap: confmap.NewFromStringMap(map[string]any{
				"metrics": map[string]any{
					"histograms": map[string]any{
						"send_count_sum_metrics": true,
					},
				},
			}),
			expectedAggrValue: true,
			warnings: []string{
				"\"metrics::histograms::send_count_sum_metrics\" has been deprecated in favor of \"metrics::histograms::send_aggregation_metrics\"",
			},
		},
		{
			name: "metrics::histograms::send_count_sum_metrics set to false",
			cfgMap: confmap.NewFromStringMap(map[string]any{
				"metrics": map[string]any{
					"histograms": map[string]any{
						"send_count_sum_metrics": false,
					},
				},
			}),
			warnings: []string{
				"\"metrics::histograms::send_count_sum_metrics\" has been deprecated in favor of \"metrics::histograms::send_aggregation_metrics\"",
			},
		},
		{
			name:              "metrics::histograms::send_count_sum_metrics and metrics::histograms::send_aggregation_metrics unset",
			cfgMap:            confmap.New(),
			expectedAggrValue: false,
		},
		{
			name: "metrics::histograms::send_aggregation_metrics set",
			cfgMap: confmap.NewFromStringMap(map[string]any{
				"metrics": map[string]any{
					"histograms": map[string]any{
						"send_aggregation_metrics": true,
					},
				},
			}),
			expectedAggrValue: true,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			f := NewFactory(nil, nil, nil, nil)
			cfg := f.CreateDefaultConfig().(*Config)
			err := component.UnmarshalConfig(testInstance.cfgMap, cfg)
			if err != nil || testInstance.err != "" {
				assert.EqualError(t, err, testInstance.err)
			} else {
				assert.Equal(t, testInstance.expectedAggrValue, cfg.Metrics.HistConfig.SendAggregations)
				var warningStr []string
				for _, warning := range cfg.warnings {
					warningStr = append(warningStr, warning.Error())
				}
				assert.ElementsMatch(t, testInstance.warnings, warningStr)
			}
		})
	}
}
