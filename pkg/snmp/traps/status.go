// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"expvar"
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

// GetStatus returns key-value data for use in status reporting of the traps server.
func GetStatus() map[string]interface{} {
	status := make(map[string]interface{})

	metricsJSON := []byte(expvar.Get("snmp_traps").String())
	metrics := make(map[string]interface{})
	json.Unmarshal(metricsJSON, &metrics) //nolint:errcheck
	status["metrics"] = metrics

	if startError != nil {
		status["error"] = startError.Error()
	}

	return status
}
