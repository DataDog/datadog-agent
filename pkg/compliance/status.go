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
	"path"

	textTemplate "text/template"

	statusComp "github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output with the collector information
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
func (Provider) JSON(stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

func (Provider) Text(buffer io.Writer) error {
	return renderText(buffer, getStatus())
}

func (Provider) HTML(buffer io.Writer) error {
	return nil
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "compliance.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("snmp").Funcs(statusComp.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
