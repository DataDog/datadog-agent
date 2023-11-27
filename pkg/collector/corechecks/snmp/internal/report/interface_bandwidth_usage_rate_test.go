// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_interfaceBandwidthState_calculateBandwidthUsageRate(t *testing.T) {
	tests := []struct {
		name             string
		symbols          []profiledefinition.SymbolConfig
		fullIndex        string
		values           *valuestore.ResultValueStore
		tags             []string
		interfaceConfigs []snmpintegration.InterfaceConfig
		expectedRate     float64
		usageValue       float64
	}{
		{
			name:      "snmp.ifBandwidthOutUsage.Rate ifHCInOctets Gauge submitted",
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
				deviceID:                mockDeviceID,
				interfaceConfigs:        tt.interfaceConfigs,
				interfaceBandwidthState: interfaceRateMapWithPrevious(),
			}

			usageName := bandwidthMetricNameToUsage[tt.symbols[0].Name]
			rate, err := ms.interfaceBandwidthState.calculateBandwidthUsageRate(ms.deviceID, tt.fullIndex, usageName, ifSpeed, tt.usageValue)
			interfaceID := mockInterfaceIDPrefix + "." + usageName

			// Expect no errors
			assert.Nil(t, err)

			assert.Equal(t, tt.expectedRate, rate)

			// Check that the map was updated with current values for next check run
			assert.Equal(t, ifSpeed, ms.interfaceBandwidthState.state[interfaceID].ifSpeed)
			assert.Equal(t, tt.usageValue, ms.interfaceBandwidthState.state[interfaceID].previousSample)
			assert.Equal(t, mockTimeNowNano, ms.interfaceBandwidthState.state[interfaceID].previousTsNano)
		})
	}
}

func Test_interfaceBandwidthState_calculateBandwidthUsageRate_logs(t *testing.T) {
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
	}{
		{
			name:      "snmp.ifBandwidthInUsage.Rate ifHCInOctets erred",
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
			expectedError: fmt.Errorf("ifSpeed changed from %d to %d for device and interface %s, no rate emitted", ifSpeed, uint64(100)*(1e6), "namespace:deviceIP:9.ifBandwidthInUsage"),
			// ((5000000 * 8) / (100 * 1000000)) * 100 = 40.0
			usageValue: 40,
			newIfSpeed: uint64(100) * (1e6),
		},
		{
			name:      "snmp.ifBandwidthOutUsage.Rate ifHCOutOctets erred",
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
			expectedError: fmt.Errorf("ifSpeed changed from %d to %d for device and interface %s, no rate emitted", ifSpeed, uint64(100)*(1e6), "namespace:deviceIP:9.ifBandwidthOutUsage"),
			// ((1000000 * 8) / (100 * 1000000)) * 100 = 8.0
			usageValue: 8,
			newIfSpeed: uint64(100) * (1e6),
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
			expectedError: fmt.Errorf("rate value for device/interface %s is negative, discarding it", "namespace:deviceIP:9.ifBandwidthInUsage"),
			// ((500000 * 8) / (100 * 1000000)) * 100 = 4.0
			usageValue: 4,
			// keep it the same interface speed, testing if the rate is negative only
			newIfSpeed: uint64(80) * (1e6),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			TimeNow = common.MockTimeNow
			ms := &MetricSender{
				sender:                  sender,
				deviceID:                mockDeviceID,
				interfaceConfigs:        tt.interfaceConfigs,
				interfaceBandwidthState: interfaceRateMapWithPrevious(),
			}
			// conflicting ifSpeed from mocked saved state (80) in interfaceRateMap
			//newIfSpeed := uint64(100) * (1e6)

			for _, symbol := range tt.symbols {
				usageName := bandwidthMetricNameToUsage[symbol.Name]
				interfaceID := mockDeviceID + ":" + fullIndex + "." + usageName
				rate, err := ms.interfaceBandwidthState.calculateBandwidthUsageRate(ms.deviceID, tt.fullIndex, usageName, tt.newIfSpeed, tt.usageValue)

				assert.Equal(t, tt.expectedError, err)
				assert.Equal(t, float64(0), rate)

				// Check that the map was updated with current values for next check run
				assert.Equal(t, tt.newIfSpeed, ms.interfaceBandwidthState.state[interfaceID].ifSpeed)
				assert.Equal(t, tt.usageValue, ms.interfaceBandwidthState.state[interfaceID].previousSample)
				assert.Equal(t, mockTimeNowNano, ms.interfaceBandwidthState.state[interfaceID].previousTsNano)
			}
		})
	}
}
