// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_interfaceBandwidthState_RemoveExpiredBandwidthUsageRates(t *testing.T) {
	tests := []struct {
		name               string
		ibs                InterfaceBandwidthState
		checkStartTimeNano int64
		expectedMapSize    int
	}{
		{
			// Map has entries from after the check's start time
			name:               "No bandwidth usage rates to remove",
			ibs:                MockInterfaceRateMap("interfaceID", 10, 10, 1, 1, mockTimeNowNano),
			checkStartTimeNano: mockTimeNowNano15SecEarlier,
			expectedMapSize:    2,
		},
		{
			// Use map with entries from 15 seconds before the check start time
			name:               "Remove expired bandwidth usage rates (entries are newer than the check's start time)",
			ibs:                interfaceRateMapWithPrevious(),
			checkStartTimeNano: mockTimeNowNano,
			expectedMapSize:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ibs.RemoveExpiredBandwidthUsageRates(tt.checkStartTimeNano)

			// Check that the map was updated to remove expired
			assert.Equal(t, tt.expectedMapSize, len(tt.ibs))
		})
	}
}

func Test_interfaceBandwidthState_calculateBandwidthUsageRate(t *testing.T) {
	tests := []struct {
		name             string
		symbol           profiledefinition.SymbolConfig
		fullIndex        string
		values           *valuestore.ResultValueStore
		tags             []string
		interfaceConfigs []snmpintegration.InterfaceConfig
		expectedRate     float64
		usageValue       float64
	}{
		{
			name:      "snmp.ifBandwidthOutUsage.Rate ifHCInOctets Gauge submitted",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
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
			// current usage value @ ts 30 for ifBandwidthInUsage
			// ((5000000 * 8) / (80 * 1000000)) * 100 = 50.0
			// previous usage value @ ts 15
			// ((3000000 * 8) / (80 * 1000000)) * 100 = 30.0
			// rate generated between ts 15 and 30
			// (50 - 30) / (30 - 15)
			expectedRate: 20.0 / 15.0,
			usageValue:   50,
		},
		{
			name:      "snmp.ifBandwidthOutUsage.Rate ifHCOutOctets submitted",
			symbol:    profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
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
			// current usage value @ ts 30 for ifBandwidthOutUsage
			// ((1000000 * 8) / (80 * 1000000)) * 100 = 10.0
			// previous usage value @ ts 15
			// ((500000 * 8) / (80 * 1000000)) * 100 = 5.0
			// rate generated between ts 15 and 30
			// (10 - 5) / (30 - 15)
			expectedRate: 5.0 / 15.0,
			usageValue:   10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			TimeNow = common.MockTimeNow
			ms := &MetricSender{
				sender:                  sender,
				interfaceConfigs:        tt.interfaceConfigs,
				interfaceBandwidthState: interfaceRateMapWithPrevious(),
			}

			usageName := bandwidthMetricNameToUsage[tt.symbol.Name]
			rate, err := ms.interfaceBandwidthState.calculateBandwidthUsageRate(tt.fullIndex, usageName, ifSpeed, tt.usageValue)
			interfaceID := fullIndex + "." + usageName

			// Expect no errors
			assert.Nil(t, err)

			assert.Equal(t, tt.expectedRate, rate)

			// Check that the map was updated with current values for next check run
			assert.Equal(t, ifSpeed, ms.interfaceBandwidthState[interfaceID].ifSpeed)
			assert.Equal(t, tt.usageValue, ms.interfaceBandwidthState[interfaceID].previousSample)
			assert.Equal(t, mockTimeNowNano, ms.interfaceBandwidthState[interfaceID].previousTsNano)
		})
	}
}

func Test_interfaceBandwidthState_calculateBandwidthUsageRate_errors(t *testing.T) {
	tests := []struct {
		name             string
		symbols          []profiledefinition.SymbolConfig
		fullIndex        string
		values           *valuestore.ResultValueStore
		tags             []string
		interfaceConfigs []snmpintegration.InterfaceConfig
		expectedError    error
		usageValue       float64
		newIfSpeed       uint64
		ibs              InterfaceBandwidthState
	}{
		{
			name:      "snmp.ifBandwidthInUsage.Rate ifHCInOctets erred when interface speed changes",
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
							Value: 100.0,
						},
					},
				},
			},
			expectedError: fmt.Errorf("ifSpeed changed from %d to %d for interface ID %s, no rate emitted", ifSpeed, uint64(100)*(1e6), "9.ifBandwidthInUsage"),
			// ((5000000 * 8) / (100 * 1000000)) * 100 = 40.0
			usageValue: 40,
			newIfSpeed: uint64(100) * (1e6),
			ibs:        interfaceRateMapWithPrevious(),
		},
		{
			name:      "snmp.ifBandwidthOutUsage.Rate ifHCOutOctets erred when interface speed changes",
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
							Value: 100.0,
						},
					},
				},
			},
			expectedError: fmt.Errorf("ifSpeed changed from %d to %d for interface ID %s, no rate emitted", ifSpeed, uint64(100)*(1e6), "9.ifBandwidthOutUsage"),
			// ((1000000 * 8) / (100 * 1000000)) * 100 = 8.0
			usageValue: 8,
			newIfSpeed: uint64(100) * (1e6),
			ibs:        interfaceRateMapWithPrevious(),
		},
		{
			name:      "snmp.ifBandwidthInUsage.Rate ifHCInOctets error on negative rate",
			symbols:   []profiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"}},
			fullIndex: "9",
			tags:      []string{"abc"},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					// ifHCInOctets
					"1.3.6.1.2.1.31.1.1.1.6": map[string]valuestore.ResultValue{
						"9": {
							Value: 500000.0,
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
							Value: 100.0,
						},
					},
				},
			},
			expectedError: fmt.Errorf("rate value for interface ID %s is negative, discarding it", "9.ifBandwidthInUsage"),
			// ((500000 * 8) / (100 * 1000000)) * 100 = 4.0
			usageValue: 4,
			// keep it the same interface speed, testing if the rate is negative only
			newIfSpeed: uint64(80) * (1e6),
			ibs:        interfaceRateMapWithPrevious(),
		},
		{
			name:      "snmp.ifBandwidthInUsage.Rate ifHCInOctets error if new entry (to not send sample)",
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
							Value: 100.0,
						},
					},
				},
			},
			expectedError: fmt.Errorf("new entry made, no rate emitted for interface ID %s", "9.ifBandwidthInUsage"),
			// ((5000000 * 8) / (100 * 1000000)) * 100 = 40.0
			usageValue: 40,
			newIfSpeed: uint64(100) * (1e6),
			ibs:        MakeInterfaceBandwidthState(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			TimeNow = common.MockTimeNow
			ms := &MetricSender{
				sender:                  sender,
				interfaceConfigs:        tt.interfaceConfigs,
				interfaceBandwidthState: tt.ibs,
			}
			for _, symbol := range tt.symbols {
				usageName := bandwidthMetricNameToUsage[symbol.Name]
				interfaceID := fullIndex + "." + usageName
				rate, err := ms.interfaceBandwidthState.calculateBandwidthUsageRate(tt.fullIndex, usageName, tt.newIfSpeed, tt.usageValue)

				assert.Equal(t, tt.expectedError, err)
				assert.Equal(t, float64(0), rate)

				// Check that the map was updated with current values for next check run
				assert.Equal(t, tt.newIfSpeed, ms.interfaceBandwidthState[interfaceID].ifSpeed)
				assert.Equal(t, tt.usageValue, ms.interfaceBandwidthState[interfaceID].previousSample)
				assert.Equal(t, mockTimeNowNano, ms.interfaceBandwidthState[interfaceID].previousTsNano)
			}
		})
	}
}
