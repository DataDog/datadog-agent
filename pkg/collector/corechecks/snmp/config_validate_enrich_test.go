package snmp

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichMetrics(t *testing.T) {
	tests := []struct {
		name            string
		metrics         []metricsConfig
		expectedErrors  []string
		expectedMetrics []metricsConfig
	}{
		{
			name: "either table symbol or scalar symbol must be provided",
			metrics: []metricsConfig{
				{},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
			expectedMetrics: []metricsConfig{
				{},
			},
		},
		{
			name: "table column symbols and scalar symbol cannot be both provided",
			metrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{},
					},
				},
			},
			expectedErrors: []string{
				"table symbol and scalar symbol cannot be both provided",
			},
		},
		{
			name: "multiple errors",
			metrics: []metricsConfig{
				{},
				{
					Symbol: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{},
					},
				},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
				"table symbol and scalar symbol cannot be both provided",
			},
		},
		{
			name: "missing symbol name",
			metrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID: "1.2.3",
					},
				},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
		},
		{
			name: "table column symbol name missing",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID: "1.2",
						},
						{
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{},
					},
				},
			},
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2`",
				"symbol oid missing: name=`abc` oid=``",
			},
		},
		{
			name: "table external metric column tag symbol error",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID: "1.2.3",
							},
						},
						metricTagConfig{
							Column: symbolConfig{
								Name: "abc",
							},
						},
					},
				},
			},
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2.3`",
				"symbol oid missing: name=`abc` oid=``",
			},
		},
		{
			name: "missing MetricTags",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{},
				},
			},
			expectedErrors: []string{
				"column symbols [{1.2 abc  <nil>}] doesn't have a 'metric_tags' section",
			},
		},
		{
			name: "table external metric column tag MIB error",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID: "1.2.3",
							},
						},
						metricTagConfig{
							Column: symbolConfig{
								Name: "abc",
							},
						},
					},
				},
			},
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2.3`",
				"symbol oid missing: name=`abc` oid=``",
			},
		},
		{
			name: "missing match tags",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z])",
						},
					},
				},
			},
			expectedErrors: []string{
				"`tags` mapping must be provided if `match` (`([a-z])`) is defined",
			},
		},
		{
			name: "match cannot compile regex",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z)",
							Tags: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			},
			expectedErrors: []string{
				"cannot compile `match` (`([a-z)`)",
			},
		},
		{
			name: "match cannot compile regex",
			metrics: []metricsConfig{
				{
					Symbols: []symbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID:  "1.2.3",
								Name: "abc",
							},
							Tag: "hello",
							IndexTransform: []metricIndexTransform{
								{
									Start: 2,
									End:   1,
								},
							},
						},
					},
				},
			},
			expectedErrors: []string{
				"transform rule end should be greater than start. Invalid rule",
			},
		},
		{
			name: "compiling extract_value",
			metrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: `(\d+)C`,
					},
				},
				{
					Symbols: []symbolConfig{
						{
							OID:          "1.2",
							Name:         "hey",
							ExtractValue: `(\d+)C`,
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID:          "1.2.3",
								Name:         "abc",
								ExtractValue: `(\d+)C`,
							},
							Tag: "hello",
						},
					},
				},
			},
			expectedMetrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID:                 "1.2.3",
						Name:                "myMetric",
						ExtractValue:        `(\d+)C`,
						extractValuePattern: regexp.MustCompile(`(\d+)C`),
					},
				},
				{
					Symbols: []symbolConfig{
						{
							OID:                 "1.2",
							Name:                "hey",
							ExtractValue:        `(\d+)C`,
							extractValuePattern: regexp.MustCompile(`(\d+)C`),
						},
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{
							Column: symbolConfig{
								OID:                 "1.2.3",
								Name:                "abc",
								ExtractValue:        `(\d+)C`,
								extractValuePattern: regexp.MustCompile(`(\d+)C`),
							},
							Tag: "hello",
						},
					},
				},
			},
			expectedErrors: []string{},
		},
		{
			name: "error compiling extract_value",
			metrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: "[{",
					},
				},
			},
			expectedErrors: []string{
				"cannot compile `extract_value`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateEnrichMetrics(tt.metrics)
			assert.Equal(t, len(tt.expectedErrors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i], tt.expectedErrors[i])
			}
			if tt.expectedMetrics != nil {
				assert.Equal(t, tt.expectedMetrics, tt.metrics)
			}
		})
	}
}
