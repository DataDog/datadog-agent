// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package status exposes the expvars we use for status tracking.
package status

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

// Manager exposes the expvars we care about
type Manager interface {
	AddTrapsPackets(int64)
	GetTrapsPackets() int64
	AddTrapsPacketsAuthErrors(int64)
	GetTrapsPacketsAuthErrors() int64
}

// New creates a new manager
func New() Manager {
	return &manager{}
}

type manager struct{}

func (s *manager) AddTrapsPackets(i int64) {
	trapsPackets.Add(i)
}

func (s *manager) AddTrapsPacketsAuthErrors(i int64) {
	trapsPacketsAuthErrors.Add(i)
}

func (s *manager) GetTrapsPackets() int64 {
	return trapsPackets.Value()
}

func (s *manager) GetTrapsPacketsAuthErrors() int64 {
	return trapsPacketsAuthErrors.Value()
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

	// if startError != nil {
	// 	status["error"] = startError.Error()
	// }
	return status
}
