// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package status exposes the expvars we use for status tracking.
package status

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
)

var (
	trapsExpvars                       = expvar.NewMap("snmp_traps")
	trapsPackets                       = expvar.Int{}
	trapsPacketsUnknownCommunityString = expvar.Int{}
)

func init() {
	trapsExpvars.Set("Packets", &trapsPackets)
	trapsExpvars.Set("PacketsUnknownCommunityString", &trapsPacketsUnknownCommunityString)
}

// Manager exposes the expvars we care about
type Manager interface {
	AddTrapsPackets(int64)
	GetTrapsPackets() int64
	AddTrapsPacketsUnknownCommunityString(int64)
	GetTrapsPacketsUnknownCommunityString() int64
}

// New creates a new manager
func New() Manager {
	return &manager{}
}

type manager struct{}

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

	return status
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// GetProvider if snamp traps is enabled returns status.Provider otherwise returns NoopProvider
func GetProvider() status.Provider {
	if config.Datadog.GetBool("network_devices.snmp_traps.enabled") {
		return Provider{}
	}

	return status.NoopProvider{}
}

// Name returns the name
func (Provider) Name() string {
	return "SNMP Traps"
}

// Section return the section
func (Provider) Section() string {
	return "SNMP Traps"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	for key, value := range GetStatus() {
		stats[key] = value
	}

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "snmp.tmpl", buffer, GetStatus())
}

// HTML renders the html output
func (Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "snmpHTML.tmpl", buffer, GetStatus())
}
