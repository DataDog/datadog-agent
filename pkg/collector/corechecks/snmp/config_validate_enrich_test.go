package snmp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichMetrics(t *testing.T) {
	tests := []struct {
		name    string
		metrics []metricsConfig
		errors  []string
	}{
		{
			name: "either table symbol or scalar symbol must be provided",
			metrics: []metricsConfig{
				{},
			},
			errors: []string{
				"either a table symbol or a scalar symbol must be provided",
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
				"column symbols [{1.2 abc}] doesn't have a 'metric_tags' section",
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
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
			errors: []string{
				"transform rule end should be greater than start. Invalid rule",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateEnrichMetrics(tt.metrics)
			assert.Equal(t, len(tt.errors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i], tt.errors[i])
			}
		})
	}
}
