// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerimpl defines the aggregator demultiplexer
package demultiplexerimpl

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

type demultiplexerStatus struct {
	Log log.Component
}

func (d demultiplexerStatus) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	d.populateStatus(stats)

	return stats
}

func (d demultiplexerStatus) populateStatus(stats map[string]interface{}) {
	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats) //nolint:errcheck
	stats["aggregatorStats"] = aggregatorStats
	s, err := checkstats.TranslateEventPlatformEventTypes(stats["aggregatorStats"])
	if err != nil {
		d.Log.Debugf("failed to translate event platform event types in aggregatorStats: %s", err.Error())
	} else {
		stats["aggregatorStats"] = s
	}
}

// Name returns the name
func (d demultiplexerStatus) Name() string {
	return "Aggregator"
}

// Section return the section
func (d demultiplexerStatus) Section() string {
	return "aggregator"
}

// JSON populates the status map
func (d demultiplexerStatus) JSON(_ bool, stats map[string]interface{}) error {
	d.populateStatus(stats)

	return nil
}

// Text renders the text output
func (d demultiplexerStatus) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "aggregator.tmpl", buffer, d.getStatusInfo())
}

// HTML renders the html output
func (d demultiplexerStatus) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "aggregatorHTML.tmpl", buffer, d.getStatusInfo())
}
