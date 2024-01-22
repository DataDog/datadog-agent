// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package statusimpl implements the Status component.
package statusimpl

import (
	"encoding/json"
	"expvar"

	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(New),
)

var (
	trapsExpvars                       = expvar.NewMap("snmp_traps")
	trapsPackets                       = expvar.Int{}
	trapsPacketsUnknownCommunityString = expvar.Int{}
	// startError stores the error we report to GetStatus()
	startError error
)

func init() {
	trapsExpvars.Set("Packets", &trapsPackets)
	trapsExpvars.Set("PacketsUnknownCommunityString", &trapsPacketsUnknownCommunityString)
}

// New creates a new status manager component
func New() status.Component {
	return &manager{}
}

type manager struct {
}

func (s *manager) AddTrapsPackets(i int64) {
	trapsPackets.Add(i)
}

func (s *manager) AddTrapsPacketsUnknownCommunityString(i int64) {
	trapsPacketsUnknownCommunityString.Add(i)
}

func (s *manager) GetTrapsPackets() int64 {
	return trapsPackets.Value()
}

func (s *manager) GetTrapsPacketsUnknownCommunityString() int64 {
	return trapsPacketsUnknownCommunityString.Value()
}

func (s *manager) GetStartError() error {
	return startError
}

func (s *manager) SetStartError(err error) {
	startError = err
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
	if dropped := getDroppedPackets(); dropped > 0 {
		metrics["PacketsDropped"] = dropped
	}
	status["metrics"] = metrics
	if startError != nil {
		status["error"] = startError.Error()
	}
	return status
}
