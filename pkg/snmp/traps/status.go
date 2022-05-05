// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
)

var (
	trapsExpvars           = expvar.NewMap("snmp_traps")
	trapsPackets           = expvar.Int{}
	trapsPacketsAuthErrors = expvar.Int{}
)

func init() {
	trapsExpvars.Set("Packets", &trapsPackets)
	trapsExpvars.Set("PacketsAuthErrors", &trapsPacketsAuthErrors)
}

func getDroppedPackets() int64 {
	aggregatorMetrics, ok := expvar.Get("aggregator").(*expvar.Map)
	if !ok {
		return 0
	}

	epErrors, ok := aggregatorMetrics.Get("EventPlatformEventsErrors").(*expvar.Map)
	if !ok {
		return 0
	}

	droppedPackets, ok := epErrors.Get(epforwarder.EventTypeSnmpTraps).(*expvar.Int)
	if !ok {
		return 0
	}
	return droppedPackets.Value()
}

// GetStatus returns key-value data for use in status reporting of the traps server.
func GetStatus() map[string]interface{} {

	status := make(map[string]interface{})

	metricsJSON := []byte(expvar.Get("snmp_traps").String())
	metrics := make(map[string]interface{})
	json.Unmarshal(metricsJSON, &metrics) //nolint:errcheck
	metrics["PacketsDropped"] = getDroppedPackets()
	status["metrics"] = metrics

	if startError != nil {
		status["error"] = startError.Error()
	}
	return status
}
