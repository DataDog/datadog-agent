// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	statusComp "github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "Compliance"
}

// Section return the section
func (Provider) Section() string {
	return "compliance"
}

func getStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats)

	return stats
}

func populateStatus(stats map[string]interface{}) {
	complianceVar := expvar.Get("compliance")
	runnerVar := expvar.Get("runner")
	if complianceVar != nil {
		complianceStatusJSON := []byte(complianceVar.String())
		complianceStatus := make(map[string]interface{})
		json.Unmarshal(complianceStatusJSON, &complianceStatus) //nolint:errcheck
		stats["complianceChecks"] = complianceStatus["Checks"]

		// This is the information from collector provider
		// Would be great to find a better pattern
		if runnerVar != nil {
			runnerStatsJSON := []byte(expvar.Get("runner").String())
			runnerStats := make(map[string]interface{})
			json.Unmarshal(runnerStatsJSON, &runnerStats) //nolint:errcheck
			stats["runnerStats"] = runnerStats
		}
	} else {
		stats["complianceChecks"] = map[string]interface{}{}
		stats["runnerStats"] = map[string]interface{}{}
	}
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return statusComp.RenderText(templatesFS, "compliance.tmpl", buffer, getStatus())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}
