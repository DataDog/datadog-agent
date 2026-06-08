// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"bytes"
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

// frameworkSummary holds aggregated check results for one framework.
type frameworkSummary struct {
	ID      string
	Version string
	Source  string
	Total   int
	Passed  int
	Failed  int
	Error   int
	Skipped int
	NotRun  int
}

// frameworkSummaries groups CheckStatus slices by framework and returns summaries sorted
// by framework ID.
func frameworkSummaries(checks []*CheckStatus) []frameworkSummary {
	byID := map[string]*frameworkSummary{}
	order := []string{}
	for _, c := range checks {
		s, ok := byID[c.Framework]
		if !ok {
			s = &frameworkSummary{ID: c.Framework, Version: c.Version, Source: c.Source}
			byID[c.Framework] = s
			order = append(order, c.Framework)
		}
		s.Total++
		if c.LastEvent == nil {
			s.NotRun++
			continue
		}
		switch c.LastEvent.Result {
		case CheckPassed:
			s.Passed++
		case CheckFailed:
			s.Failed++
		case CheckError:
			s.Error++
		case CheckSkipped:
			s.Skipped++
		default:
			s.NotRun++
		}
	}
	result := make([]frameworkSummary, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result
}

// RenderStatusText renders the compliance status to text using the standard template.
func (a *Agent) RenderStatusText() (string, error) {
	var buf bytes.Buffer
	if err := statusComp.RenderText(templatesFS, "compliance.tmpl", &buf, a.summaryStatusData()); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// summaryStatusData returns status data with per-framework summaries for the remote agent text view.
func (a *Agent) summaryStatusData() map[string]interface{} {
	complianceStats := map[string]interface{}{
		"endpoints":          a.opts.Reporter.Endpoints().GetStatus(),
		"frameworkSummaries": frameworkSummaries(a.getChecksStatus()),
	}
	return map[string]interface{}{
		"complianceStatus": complianceStats,
	}
}

// StatusData returns the compliance status as a map suitable for JSON serialization.
func (a *Agent) StatusData() map[string]interface{} {
	complianceStats := map[string]interface{}{
		"endpoints":        a.opts.Reporter.Endpoints().GetStatus(),
		"complianceChecks": a.getChecksStatus(),
		"runnerStats":      map[string]interface{}{},
	}

	if runnerVar := expvar.Get("runner"); runnerVar != nil {
		runnerStats := make(map[string]interface{})
		if err := json.Unmarshal([]byte(runnerVar.String()), &runnerStats); err == nil {
			complianceStats["runnerStats"] = runnerStats
		}
	}

	return map[string]interface{}{
		"complianceStatus": complianceStats,
	}
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	for k, v := range s.agent.StatusData() {
		stats[k] = v
	}
}

func (s statusProvider) getStatus() map[string]interface{} {
	return s.agent.StatusData()
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
