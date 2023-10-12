// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_metricSender_sendBandwidthUsageMetric(t *testing.T) {
	type Metric struct {
		name  string
		value float64
	}
	tests := []struct {
		name             string
		symbols          []profiledefinition.SymbolConfig
		fullIndex        string
		values           *valuestore.ResultValueStore
		tags             []string
		interfaceConfigs []snmpintegration.InterfaceConfig
		expectedMetric   []Metric
		expectedError    error
	}{
		{
			name:      "snmp.ifBandwidthInUsage.Rate submitted",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			tags:      []string{"abc"},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				// ((5000000 * 8) / (80 * 1000000)) * 100 = 50.0
				{"snmp.ifBandwidthInUsage.rate", 50.0},
			},
		},
		{
			name:      "snmp.ifBandwidthOutUsage.Rate submitted",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				// ((1000000 * 8) / (80 * 1000000)) * 100 = 10.0
				{"snmp.ifBandwidthOutUsage.rate", 10.0},
			},
		},
		{
			name:      "not a bandwidth metric",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.99", Name: "notABandwidthMetric"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{},
			},
			expectedMetric: []Metric{},
		},
		{
			name:      "missing ifHighSpeed",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("bandwidth usage: missing `ifHighSpeed` metric, skipping metric. fullIndex=9"),
		},
		{
			name:      "missing ifHCInOctets",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("bandwidth usage: missing `ifHCInOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			name:      "missing ifHCOutOctets",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCOutOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("bandwidth usage: missing `ifHCOutOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			name:      "missing ifHCInOctets value",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9999": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("bandwidth usage: missing value for `ifHCInOctets` metric, skipping this row. fullIndex=9"),
		},
		{
			name:      "missing ifHighSpeed value",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"999": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("bandwidth usage: missing value for `ifHighSpeed`, skipping this row. fullIndex=9"),
		},
		{
			name:      "cannot convert ifHighSpeed to float",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: "abc",
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("failed to convert ifHighSpeedValue to float64: failed to parse `abc`: strconv.ParseFloat: parsing \"abc\": invalid syntax"),
		},
		{
			name:      "cannot convert ifHCInOctets to float",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: "abc",
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{},
			expectedError:  fmt.Errorf("failed to convert octetsValue to float64: failed to parse `abc`: strconv.ParseFloat: parsing \"abc\": invalid syntax"),
		},
		{
			name: "[custom speed] snmp.ifBandwidthIn/OutUsage.rate with custom interface speed matched by name",
			symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
				{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
			},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "name",
				MatchValue: "eth0",
				InSpeed:    160_000_000,
				OutSpeed:   40_000_000,
				Tags:       []string{"muted", "customIfTagKey:customIfTagValue"},
			}},
			tags: []string{
				"interface:eth0",
				"muted",
				"customIfTagKey:customIfTagValue",
			},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				// ((5000000 * 8) / (160 * 1000000)) * 100 = 25.0
				{"snmp.ifBandwidthInUsage.rate", 25.0},
				// ((1000000 * 8) / (40 * 1000000)) * 100 = 20.0
				{"snmp.ifBandwidthOutUsage.rate", 20.0},
			},
		},
		{
			name: "[custom speed] snmp.ifBandwidthIn/OutUsage.rate with custom interface speed matched by index",
			symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
				{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
			},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "index",
				MatchValue: "9",
				InSpeed:    160_000_000,
				OutSpeed:   40_000_000,
				Tags:       []string{"muted", "customIfTagKey:customIfTagValue"},
			}},
			tags: []string{
				"interface:eth0",
				"muted",
				"customIfTagKey:customIfTagValue",
			},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				// ((5000000 * 8) / (160 * 1000000)) * 100 = 25.0
				{"snmp.ifBandwidthInUsage.rate", 25.0},
				// ((1000000 * 8) / (40 * 1000000)) * 100 = 20.0
				{"snmp.ifBandwidthOutUsage.rate", 20.0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &MetricSender{
				sender:           sender,
				interfaceConfigs: tt.interfaceConfigs,
			}
			for _, symbol := range tt.symbols {
				err := ms.sendBandwidthUsageMetric(symbol, tt.fullIndex, tt.values, tt.tags)
				assert.Equal(t, tt.expectedError, err)
			}

			for _, metric := range tt.expectedMetric {
				sender.AssertMetric(t, "Rate", metric.name, metric.value, "", tt.tags)
			}
		})
	}
}

func Test_metricSender_sendIfSpeedMetrics(t *testing.T) {
	type Metric struct {
		name  string
		value float64
		tags  []string
	}
	tests := []struct {
		name             string
		symbol           profiledefinition.SymbolConfig
		fullIndex        string
		values           *valuestore.ResultValueStore
		tags             []string
		interfaceConfigs []snmpintegration.InterfaceConfig
		expectedMetric   []Metric
	}{
		{
			name:      "InSpeed and OutSpeed Override",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "index",
				MatchValue: "9",
				InSpeed:    160_000_000,
				OutSpeed:   40_000_000,
			}},
			tags: []string{"interface:eth0"},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				{"snmp.ifInSpeed", 160_000_000, []string{"interface:eth0", "speed_source:custom"}},
				{"snmp.ifOutSpeed", 40_000_000, []string{"interface:eth0", "speed_source:custom"}},
			},
		},
		{
			name:      "InSpeed and OutSpeed Override with custom tags",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "index",
				MatchValue: "9",
				InSpeed:    160_000_000,
				OutSpeed:   40_000_000,
				Tags:       []string{"muted", "customKey:customValue"},
			}},
			tags: []string{"interface:eth0"},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				{"snmp.ifInSpeed", 160_000_000, []string{"interface:eth0", "speed_source:custom", "muted", "customKey:customValue"}},
				{"snmp.ifOutSpeed", 40_000_000, []string{"interface:eth0", "speed_source:custom", "muted", "customKey:customValue"}},
			},
		},
		{
			name:      "InSpeed Override but not OutSpeed Override",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "index",
				MatchValue: "9",
				InSpeed:    160_000_000,
			}},
			tags: []string{"interface:eth0"},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				{"snmp.ifInSpeed", 160_000_000, []string{"interface:eth0", "speed_source:custom"}},
				{"snmp.ifOutSpeed", 80_000_000, []string{"interface:eth0", "speed_source:device"}},
			},
		},
		{
			name:      "InSpeed and OutSpeed config with zero values",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex: "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{{
				MatchField: "index",
				MatchValue: "9",
				InSpeed:    0,
				OutSpeed:   0,
			}},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				{"snmp.ifInSpeed", 80_000_000, []string{"speed_source:device"}},
				{"snmp.ifOutSpeed", 80_000_000, []string{"speed_source:device"}},
			},
		},
		{
			name:             "no interface config found",
			symbol:           profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex:        "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			expectedMetric: []Metric{
				{"snmp.ifInSpeed", 80_000_000, []string{"speed_source:device"}},
				{"snmp.ifOutSpeed", 80_000_000, []string{"speed_source:device"}},
			},
		},
		{
			name:             "no interface config found and no ifHighSpeed",
			symbol:           profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			fullIndex:        "9",
			interfaceConfigs: []snmpintegration.InterfaceConfig{},
			values:           &valuestore.ResultValueStore{},
			expectedMetric:   []Metric{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &MetricSender{
				sender:           sender,
				interfaceConfigs: tt.interfaceConfigs,
			}
			ms.sendIfSpeedMetrics(tt.symbol, tt.fullIndex, tt.values, tt.tags)

			for _, metric := range tt.expectedMetric {
				sender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}
			assert.Equal(t, len(tt.expectedMetric), len(sender.Mock.Calls))
		})
	}
}

func Test_metricSender_sendInterfaceVolumeMetrics(t *testing.T) {
	type Metric struct {
		metricMethod string
		name         string
		value        float64
	}
	tests := []struct {
		name           string
		symbol         profiledefinition.SymbolConfig
		fullIndex      string
		values         *valuestore.ResultValueStore
		expectedMetric []Metric
	}{
		{
			"snmp.ifBandwidthInUsage.Rate submitted",
			profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"9": {
							Value: 80.0,
						},
					},
				},
			},
			[]Metric{
				// ((5000000 * 8) / (80 * 1000000)) * 100 = 50.0
				{"Rate", "snmp.ifBandwidthInUsage.rate", 50.0},
				{"Gauge", "snmp.ifInSpeed", 80_000_000},
				{"Gauge", "snmp.ifOutSpeed", 80_000_000},
			},
		},
		{
			"should complete even on error",
			profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
			"9",
			&valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 5000000.0,
						},
					},
					// ifHCOutOctets
					"1.3.6.1.2.1.31.1.1.1.10": map[string]valuestore.ResultValue{
						"9": {
							Value: 1000000.0,
						},
					},
					// ifHighSpeed
					"1.3.6.1.2.1.31.1.1.1.15": map[string]valuestore.ResultValue{
						"999": {
							Value: 80.0,
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
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &MetricSender{
				sender: sender,
			}
			tags := []string{"foo:bar"}
			ms.sendInterfaceVolumeMetrics(tt.symbol, tt.fullIndex, tt.values, tags)

			for _, metric := range tt.expectedMetric {
				sender.AssertMetric(t, metric.metricMethod, metric.name, metric.value, "", tags)
			}
		})
	}
}
