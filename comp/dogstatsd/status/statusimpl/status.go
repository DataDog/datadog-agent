// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type provides struct {
	fx.Out

	StatusProvider status.InformationProvider
}

// Module defines the fx options for the status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus))
}

type statusProvider struct {
	Config config.Component
}

func newStatus() provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{}),
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s statusProvider) Name() string {
	return "Dogstatsd"
}

// Section returns the section
func (s statusProvider) Section() string {
	return "dogstatsd"
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "dogstatsd.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "dogstatsdHTML.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	s.populateStatus(stats)

	return stats
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	if expvar.Get("dogstatsd") != nil {
		dogstatsdStatsJSON := []byte(expvar.Get("dogstatsd").String())
		dogstatsdUdsStatsJSON := []byte(expvar.Get("dogstatsd-uds").String())
		dogstatsdUDPStatsJSON := []byte(expvar.Get("dogstatsd-udp").String())
		dogstatsdStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdStatsJSON, &dogstatsdStats) //nolint:errcheck
		dogstatsdUdsStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdUdsStatsJSON, &dogstatsdUdsStats) //nolint:errcheck
		for name, value := range dogstatsdUdsStats {
			dogstatsdStats["Uds"+name] = value
		}
		dogstatsdUDPStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdUDPStatsJSON, &dogstatsdUDPStats) //nolint:errcheck
		for name, value := range dogstatsdUDPStats {
			dogstatsdStats["Udp"+name] = value
		}
		stats["dogstatsdStats"] = dogstatsdStats
	}
}
