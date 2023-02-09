// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

var bandwidthMetricNameToUsage = map[string]string{
	"ifHCInOctets":  "ifBandwidthInUsage",
	"ifHCOutOctets": "ifBandwidthOutUsage",
}

const ifHighSpeedOID = "1.3.6.1.2.1.31.1.1.1.15"

func (ms *MetricSender) trySendBandwidthUsageMetric(symbol checkconfig.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string, interfaceConfig *checkconfig.InterfaceConfig) {
	err := ms.sendBandwidthUsageMetric(symbol, fullIndex, values, tags, interfaceConfig)
	if err != nil {
		log.Debugf("failed to send bandwidth usage metric: %s", err)
	}
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
func (ms *MetricSender) sendBandwidthUsageMetric(symbol checkconfig.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string, interfaceConfig *checkconfig.InterfaceConfig) error {
	usageName, ok := bandwidthMetricNameToUsage[symbol.Name]
	if !ok {
		return nil
	}
	var ifHighSpeedFloatValue float64
	if interfaceConfig != nil {
		switch symbol.Name {
		case "ifHCInOctets":
			ifHighSpeedFloatValue = float64(interfaceConfig.InSpeed)
		case "ifHCOutOctets":
			ifHighSpeedFloatValue = float64(interfaceConfig.OutSpeed)
		default:
			// TODO: DO SOMETHING?
			// not expected case.
		}
	} else {
		ifHighSpeedValues, err := values.GetColumnValues(ifHighSpeedOID)
		if err != nil {
			return fmt.Errorf("bandwidth usage: missing `ifHighSpeed` metric, skipping metric. fullIndex=%s", fullIndex)
		}
		ifHighSpeedValue, ok := ifHighSpeedValues[fullIndex]
		if !ok {
			return fmt.Errorf("bandwidth usage: missing value for `ifHighSpeed`, skipping this row. fullIndex=%s", fullIndex)
		}

		ifHighSpeedFloatValue, err = ifHighSpeedValue.ToFloat64()
		if err != nil {
			return fmt.Errorf("failed to convert ifHighSpeedValue to float64: %s", err)
		}
		if ifHighSpeedFloatValue == 0.0 {
			return fmt.Errorf("bandwidth usage: zero or invalid value for ifHighSpeed, skipping this row. fullIndex=%s, ifHighSpeedValue=%#v", fullIndex, ifHighSpeedValue)
		}
	}
	log.Warnf("ifHighSpeedFloatValue: %.2f", ifHighSpeedFloatValue)

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
	usageValue := ((octetsFloatValue * 8) / (ifHighSpeedFloatValue * (1e6))) * 100.0

	sample := MetricSample{
		value:      valuestore.ResultValue{SubmissionType: "counter", Value: usageValue},
		tags:       tags,
		symbol:     checkconfig.SymbolConfig{Name: usageName + ".rate"},
		forcedType: "counter",
		options:    checkconfig.MetricsConfigOption{},
	}

	ms.sendMetric(sample)
	return nil
}
