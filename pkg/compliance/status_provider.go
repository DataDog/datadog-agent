// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

type statusProvider struct {
	agent *Agent
}

// StatusProvider returns the compliance status provider
func (a *Agent) StatusProvider() statusComp.Provider {
	return statusProvider{
		agent: a,
	}
}

// Name returns the name
func (statusProvider) Name() string {
	return "Compliance"
}

// Section return the section
func (statusProvider) Section() string {
	return "compliance"
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	complianceStats := map[string]interface{}{}

	complianceStats["endpoints"] = s.agent.opts.Reporter.Endpoints().GetStatus()

	complianceVar := expvar.Get("compliance")
	runnerVar := expvar.Get("runner")
	if complianceVar != nil {
		complianceStatusJSON := []byte(complianceVar.String())
		complianceStatus := make(map[string]interface{})
		json.Unmarshal(complianceStatusJSON, &complianceStatus) //nolint:errcheck
		complianceStats["complianceChecks"] = complianceStatus["Checks"]

		// This is the information from collector provider
		// Would be great to find a better pattern
		if runnerVar != nil {
			runnerStatsJSON := []byte(expvar.Get("runner").String())
			runnerStats := make(map[string]interface{})
			json.Unmarshal(runnerStatsJSON, &runnerStats) //nolint:errcheck
			complianceStats["runnerStats"] = runnerStats
		}
	} else {
		complianceStats["complianceChecks"] = map[string]interface{}{}
		complianceStats["runnerStats"] = map[string]interface{}{}
	}

	stats["complianceStatus"] = complianceStats
}

func (s statusProvider) getStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	s.populateStatus(stats)

	return stats
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return statusComp.RenderText(templatesFS, "compliance.tmpl", buffer, s.getStatus())
}

// HTML renders the html output
func (statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
