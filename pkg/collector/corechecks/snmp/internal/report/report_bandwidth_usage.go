// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package report

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

// TODO: Rename file to report_interface_volume_metrics.go in a separate PR.
//       Making the change in the current PR will make review harder (it makes the whole file considered as deleted).

// TimeNow is the unix time to use for rate (delta) calculations
var TimeNow = time.Now

var bandwidthMetricNameToUsage = map[string]string{
	"ifHCInOctets":  "ifBandwidthInUsage",
	"ifHCOutOctets": "ifBandwidthOutUsage",
}

const ifHighSpeedOID = "1.3.6.1.2.1.31.1.1.1.15"

// sendInterfaceVolumeMetrics is responsible for handling special interface related metrics like:
//   - bandwidth usage metric
//   - if speed metrics based on custom interface speed and ifHighSpeed
func (ms *MetricSender) sendInterfaceVolumeMetrics(symbol profiledefinition.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string) {
	err := ms.sendBandwidthUsageMetric(symbol, fullIndex, values, tags)
	if err != nil {
		log.Debugf("failed to send bandwidth usage metric: %s", err)
	}

	ms.sendIfSpeedMetrics(symbol, fullIndex, values, tags)
}

/*
sendBandwidthUsageMetric evaluate and report input/output bandwidth usage.
If any of `ifHCInOctets`, `ifHCOutOctets`  or `ifHighSpeed` is missing then bandwidth will not be reported.

Bandwidth usage is:

interface[In|Out]Octets(t+dt) - interface[In|Out]Octets(t)
----------------------------------------------------------
dt*interfaceSpeed

Given:
* ifHCInOctets: the total number of octets received on the interface.
* ifHCOutOctets: The total number of octets transmitted out of the interface.
* ifHighSpeed: An estimate of the interface's current bandwidth in Mb/s (10^6 bits
per second). It is constant in time, can be overwritten by the system admin.
It is the total available bandwidth.
Bandwidth usage is evaluated as: ifHC[In|Out]Octets/ifHighSpeed and reported as *Rate*
*/
func (ms *MetricSender) sendBandwidthUsageMetric(symbol profiledefinition.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string) error {
	usageName, ok := bandwidthMetricNameToUsage[symbol.Name]
	if !ok {
		return nil
	}
	var ifSpeed uint64

	interfaceConfig, err := getInterfaceConfig(ms.interfaceConfigs, fullIndex, tags)
	if err == nil {
		switch symbol.Name {
		case "ifHCInOctets":
			ifSpeed = interfaceConfig.InSpeed
		case "ifHCOutOctets":
			ifSpeed = interfaceConfig.OutSpeed
		}
		tags = append(tags, interfaceConfig.Tags...)
	}
	if ifSpeed == 0 {
		ifHighSpeed, err := ms.getIfHighSpeed(fullIndex, values)
		if err != nil {
			return err
		}
		ifSpeed = ifHighSpeed
	}

	metricValues, err := getColumnValueFromSymbol(values, symbol)
	if err != nil {
		return fmt.Errorf("bandwidth usage: missing `%s` metric, skipping this row. fullIndex=%s", symbol.Name, fullIndex)
	}

	octetsValue, ok := metricValues[fullIndex]
	if !ok {
		return fmt.Errorf("bandwidth usage: missing value for `%s` metric, skipping this row. fullIndex=%s", symbol.Name, fullIndex)
	}

	octetsFloatValue, err := octetsValue.ToFloat64()
	if err != nil {
		return fmt.Errorf("failed to convert octetsValue to float64: %s", err)
	}
	usageValue := ((octetsFloatValue * 8) / (float64(ifSpeed))) * 100.0

	interfaceID := ms.deviceID + ":" + fullIndex + "." + usageName
	rate, err := ms.interfaceBandwidthState.calculateBandwidthUsageRate(interfaceID, ifSpeed, usageValue)
	if err != nil {
		return err
	}
	sample := MetricSample{
		value:      valuestore.ResultValue{SubmissionType: profiledefinition.ProfileMetricTypeGauge, Value: rate},
		tags:       tags,
		symbol:     profiledefinition.SymbolConfig{Name: usageName + ".rate"},
		forcedType: profiledefinition.ProfileMetricTypeGauge,
		options:    profiledefinition.MetricsConfigOption{},
	}
	ms.sendMetric(sample)
	return nil
}

/*
calculateRate is responsible for checking the state for previously seen metric sample to generate the rate from.
If the ifSpeed has changed for the interface, the rate will not be submitted (drop the previous sample)
*/
func (ibs *InterfaceBandwidthState) calculateBandwidthUsageRate(interfaceID string, ifSpeed uint64, usageValue float64) (float64, error) {
	// current data point has the same interface speed as last data point
	ibs.mu.RLock()
	state, ok := ibs.state[interfaceID]
	ibs.mu.RUnlock()
	if ok && state.ifSpeed == ifSpeed {
		// Get time in seconds with nanosecond precision, as core agent uses for rate calculation in aggregator
		// https://github.com/DataDog/datadog-agent/blob/ecedf4648f41193988b4727fc6f893a0dfc3991e/pkg/aggregator/aggregator.go#L96
		currentTsNano := TimeNow().UnixNano()
		currentTs := float64(currentTsNano) / float64(time.Second)
		prevTs := float64(state.previousTsNano) / float64(time.Second)

		// calculate the delta, taken from pkg/metrics/rate.go
		// https://github.com/DataDog/datadog-agent/blob/ecedf4648f41193988b4727fc6f893a0dfc3991e/pkg/metrics/rate.go#L37
		delta := (usageValue - state.previousSample) / (currentTs - prevTs)

		// update the map previous as the current for next rate
		state.previousSample = usageValue
		state.previousTsNano = currentTsNano
		ibs.mu.Lock()
		ibs.state[interfaceID] = state
		ibs.mu.Unlock()

		if delta < 0 {
			return 0, fmt.Errorf("Rate value for device/interface %s is negative, discarding it", interfaceID)
		}
		return delta, nil
	}
	// otherwise, no previous data point / different ifSpeed - make new entry for interface
	ibs.mu.Lock()
	ibs.state[interfaceID] = BandwidthUsage{
		ifSpeed:        ifSpeed,
		previousSample: usageValue,
		previousTsNano: TimeNow().UnixNano(),
	}
	ibs.mu.Unlock()
	// do not send a sample to metrics, send error for ifSpeed change (previous entry conflicted)
	if ok {
		log.Infof("ifSpeed changed from %d to %d for device and interface %s, no rate emitted", state.ifSpeed, ifSpeed, interfaceID)
	}
	return 0, nil
}

func (ms *MetricSender) sendIfSpeedMetrics(symbol profiledefinition.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string) {
	// We are piggybacking on presence of ifHCInOctets as a way to 1/ submit ifSpeed metrics only once, 2/ have corresponding fullIndex and 3/ tags.
	// If needed, we can improve (at cost of complexity) by sending ifSpeed metrics based on presence of multiple metrics like ifHCInOctets/ifHCOutOctets/ifHighSpeed.
	// I think it's reasonable for now, for simplify, to only rely on ifHCInOctets that should be present in the vast majority of cases.
	if symbol.Name != "ifHCInOctets" {
		return
	}
	interfaceConfig, err := getInterfaceConfig(ms.interfaceConfigs, fullIndex, tags)
	if err != nil {
		log.Tracef("continue with empty interfaceConfig: %s", err)
		interfaceConfig = snmpintegration.InterfaceConfig{}
	}
	tags = append(tags, interfaceConfig.Tags...)

	ifHighSpeed, err := ms.getIfHighSpeed(fullIndex, values)
	if err != nil {
		log.Tracef("continue with empty interfaceConfig: %s", err)
		ifHighSpeed = 0
	}
	ms.sendIfSpeedMetric("ifInSpeed", interfaceConfig.InSpeed, ifHighSpeed, tags)
	ms.sendIfSpeedMetric("ifOutSpeed", interfaceConfig.OutSpeed, ifHighSpeed, tags)
}

func (ms *MetricSender) sendIfSpeedMetric(symbolName string, customSpeed uint64, ifHighSpeed uint64, tags []string) {
	ifSpeed := customSpeed
	speedSource := "custom"
	if customSpeed == 0 {
		ifSpeed = ifHighSpeed
		speedSource = "device"
	}
	if ifSpeed == 0 {
		return
	}

	newTags := append([]string{"speed_source:" + speedSource}, tags...)
	ms.sendMetric(MetricSample{
		value:      valuestore.ResultValue{Value: float64(ifSpeed)},
		tags:       newTags,
		symbol:     profiledefinition.SymbolConfig{Name: symbolName},
		forcedType: profiledefinition.ProfileMetricTypeGauge,
		options:    profiledefinition.MetricsConfigOption{},
	})
}

// getIfHighSpeed returns getIfHighSpeed collected via SNMP
func (ms *MetricSender) getIfHighSpeed(fullIndex string, values *valuestore.ResultValueStore) (uint64, error) {
	ifHighSpeedValues, err := values.GetColumnValues(ifHighSpeedOID)
	if err != nil {
		return 0, fmt.Errorf("bandwidth usage: missing `ifHighSpeed` metric, skipping metric. fullIndex=%s", fullIndex)
	}
	ifHighSpeedValue, ok := ifHighSpeedValues[fullIndex]
	if !ok {
		return 0, fmt.Errorf("bandwidth usage: missing value for `ifHighSpeed`, skipping this row. fullIndex=%s", fullIndex)
	}

	ifHighSpeedFloatValue, err := ifHighSpeedValue.ToFloat64()
	if err != nil {
		return 0, fmt.Errorf("failed to convert ifHighSpeedValue to float64: %s", err)
	}
	if ifHighSpeedFloatValue == 0.0 {
		return 0, fmt.Errorf("bandwidth usage: zero or invalid value for ifHighSpeed, skipping this row. fullIndex=%s, ifHighSpeedValue=%#v", fullIndex, ifHighSpeedValue)
	}
	return uint64(ifHighSpeedFloatValue) * (1e6), nil
}
