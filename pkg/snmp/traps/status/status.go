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
	"path"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
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

	return status
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output with the collector information
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
func (Provider) JSON(stats map[string]interface{}) error {
	for key, value := range GetStatus() {
		stats[key] = value
	}

	return nil
}

func (Provider) Text(buffer io.Writer) error {
	return renderText(buffer, GetStatus())
}

func (Provider) HTML(buffer io.Writer) error {
	return renderHTML(buffer, GetStatus())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "snmpHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("snmpHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "snmp.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("snmp").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
