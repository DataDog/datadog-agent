// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package statusimpl implements the Status component.
package statusimpl

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/status"
	trapsStatus "github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
func New() trapsStatus.Component {
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

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

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
	stats["snmpTrapsStats"] = GetStatus()

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "snmp.tmpl", buffer, p.populateStatus())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "snmpHTML.tmpl", buffer, p.populateStatus())
}

func (p Provider) populateStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	p.JSON(false, stats) //nolint:errcheck

	return stats
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

	metricsJSON := []byte(trapsExpvars.String())
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
