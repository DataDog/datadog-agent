package snmp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func Test_metricSender_sendBandwidthUsageMetric(t *testing.T) {
	type Metric struct {
		name  string
		value float64
	}
	tests := []struct {
		name           string
		symbol         symbolConfig
		fullIndex      string
		values         *resultValueStore
		expectedMetric []Metric
		expectedError  error
	}{
		{
			"snmp.ifBandwidthInUsage.rate submitted",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{
				// ((5000000 * 8) / (80 * 1000000)) * 100 = 50.0
				{"snmp.ifBandwidthInUsage.rate", 50.0},
			},
			nil,
		},
		{
			"snmp.ifBandwidthOutUsage.rate submitted",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{
				// ((1000000 * 8) / (80 * 1000000)) * 100 = 10.0
				{"snmp.ifBandwidthOutUsage.rate", 10.0},
			},
			nil,
		},
		{
			"not a bandwidth metric",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.99", Name: "notABandwidthMetric"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{},
			},
			[]Metric{},
			nil,
		},
		{
			"missing ifHighSpeed",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("bandwidth usage: missing `ifHighSpeed` metric, skipping metric. fullIndex=9"),
		},
		{
			"missing ifHCInOctets",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("bandwidth usage: missing `ifHCInOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			"missing ifHCOutOctets",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCOutOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("bandwidth usage: missing `ifHCOutOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			"missing ifHCInOctets value",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9999": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("bandwidth usage: missing value for `ifHCInOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			"missing ifHighSpeed value",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"999": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("bandwidth usage: missing value for `ifHighSpeed`, skipping this row. fullIndex=9"),
		},
		{
			"cannot convert ifHighSpeed to float",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: "abc",
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("failed to convert ifHighSpeedValue to float64: failed to parse `abc`: strconv.ParseInt: parsing \"abc\": invalid syntax"),
		},
		{
			"cannot convert ifHCInOctets to float",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: "abc",
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
			fmt.Errorf("failed to convert octetsValue to float64: failed to parse `abc`: strconv.ParseInt: parsing \"abc\": invalid syntax"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &metricSender{
				sender: sender,
			}
			tags := []string{"foo:bar"}
			err := ms.sendBandwidthUsageMetric(tt.symbol, tt.fullIndex, tt.values, tags)
			assert.Equal(t, tt.expectedError, err)

			for _, metric := range tt.expectedMetric {
				sender.AssertMetric(t, "Rate", metric.name, metric.value, "", tags)
			}
		})
	}
}
func Test_metricSender_trySendBandwidthUsageMetric(t *testing.T) {
	type Metric struct {
		name  string
		value float64
	}
	tests := []struct {
		name           string
		symbol         symbolConfig
		fullIndex      string
		values         *resultValueStore
		expectedMetric []Metric
	}{
		{
			"snmp.ifBandwidthInUsage.rate submitted",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"9": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{
				// ((5000000 * 8) / (80 * 1000000)) * 100 = 50.0
				{"snmp.ifBandwidthInUsage.rate", 50.0},
			},
		},
		{
			"should complete even on error",
			symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&resultValueStore{
				columnValues: columnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]snmpValueType{
						"9": {
							value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]snmpValueType{
						"9": {
							value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]snmpValueType{
						"999": {
							value: 80.0,
						},
					},
				},
			},
			[]Metric{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &metricSender{
				sender: sender,
			}
			tags := []string{"foo:bar"}
			ms.trySendBandwidthUsageMetric(tt.symbol, tt.fullIndex, tt.values, tags)

			for _, metric := range tt.expectedMetric {
				sender.AssertMetric(t, "Rate", metric.name, metric.value, "", tags)
			}
		})
	}
}
