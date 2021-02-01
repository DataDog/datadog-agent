package snmp

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_validateMetrics(t *testing.T) {
	tests := []struct {
		name    string
		metrics []metricsConfig
		errors  []string
	}{
		{
			name: "MIB must be provided",
			metrics: []metricsConfig{
				{
					Symbol: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
				},
			},
			errors: []string{
				"MIB must be provided",
			},
		},
		{
			name: "either table symbol or scalar symbol must be provided",
			metrics: []metricsConfig{
				{
					MIB: "MY-MIB",
				},
			},
			errors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
		},
		{
			name: "table column symbols and scalar symbol cannot be both provided",
			metrics: []metricsConfig{
				{
					MIB: "MY-MIB",
					Symbol: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Table: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					MetricTags: metricTagConfigList{
						metricTagConfig{},
					},
				},
			},
			errors: []string{
				"table symbol and scalar symbol cannot be both provided",
				"when using table, a list of column symbols must be provided",
			},
		},
		{
			name: "multiple errors",
			metrics: []metricsConfig{
				{
					MIB: "MY-MIB",
				},
				{
					MIB: "MY-MIB",
					Symbol: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Table: symbolConfig{
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
					MIB: "MY-MIB",
					Symbol: symbolConfig{
						OID: "1.2.3",
					},
				},
			},
			errors: []string{
				"symbol name missing: name=`` oid=`1.2.3`",
			},
		},
		{
			name: "table column symbol name missing",
			metrics: []metricsConfig{
				{
					MIB: "mib",
					Table: symbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
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
					MIB: "mib",
					Table: symbolConfig{
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
			name: "table external metric column tag MIB error",
			metrics: []metricsConfig{
				{
					MIB: "mib",
					Table: symbolConfig{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateMetrics(tt.metrics)
			assert.Equal(t, len(tt.errors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i].Error(), tt.errors[i])
			}
		})
	}
}
