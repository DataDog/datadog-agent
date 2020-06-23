// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:generate go-bindata -pkg status -prefix templates -o ./templates.go templates/...
//go:generate go fmt ./templates.go

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var fmap = Textfmap()

// FormatStatus takes a json bytestring and prints out the formatted statuspage
func FormatStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats) //nolint:errcheck
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	pythonInit := stats["pythonInit"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	aggregatorStats := stats["aggregatorStats"]
	dogstatsdStats := stats["dogstatsdStats"]
	logsStats := stats["logsStats"]
	dcaStats := stats["clusterAgentStatus"]
	endpointsInfos := stats["endpointsInfos"]
	inventoriesStats := stats["inventories"]
	systemProbeStats := stats["systemProbeStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderStatusTemplate(b, "/header.tmpl", stats)
	renderChecksStats(b, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats, "")
	renderStatusTemplate(b, "/jmxfetch.tmpl", stats)
	renderStatusTemplate(b, "/forwarder.tmpl", forwarderStats)
	renderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos)
	renderStatusTemplate(b, "/logsagent.tmpl", logsStats)
	if config.Datadog.GetBool("system_probe_config.enabled") {
		renderStatusTemplate(b, "/systemprobe.tmpl", systemProbeStats)
	}
	renderStatusTemplate(b, "/trace-agent.tmpl", stats["apmStats"])
	renderStatusTemplate(b, "/aggregator.tmpl", aggregatorStats)
	renderStatusTemplate(b, "/dogstatsd.tmpl", dogstatsdStats)
	if config.Datadog.GetBool("cluster_agent.enabled") || config.Datadog.GetBool("cluster_checks.enabled") {
		renderStatusTemplate(b, "/clusteragent.tmpl", dcaStats)
	}

	return b.String(), nil
}

// FormatDCAStatus takes a json bytestring and prints out the formatted statuspage
func FormatDCAStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats) //nolint:errcheck
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	endpointsInfos := stats["endpointsInfos"]
	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", stats["version"])
	stats["title"] = title
	renderStatusTemplate(b, "/header.tmpl", stats)
	renderChecksStats(b, runnerStats, nil, nil, autoConfigStats, checkSchedulerStats, nil, "")
	renderStatusTemplate(b, "/forwarder.tmpl", forwarderStats)
	renderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos)

	return b.String(), nil
}

// FormatHPAStatus takes a json bytestring and prints out the formatted statuspage
func FormatHPAStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)
	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats) //nolint:errcheck
	renderStatusTemplate(b, "/custommetricsprovider.tmpl", stats)
	return b.String(), nil
}

// FormatSecurityAgentStatus takes a json bytestring and prints out the formatted status for security agent
func FormatSecurityAgentStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats) //nolint:errcheck
	runnerStats := stats["runnerStats"]
	title := fmt.Sprintf("Datadog Security Agent (v%s)", stats["version"])
	stats["title"] = title
	renderStatusTemplate(b, "/header.tmpl", stats)
	renderComplianceChecksStats(b, runnerStats)

	return b.String(), nil
}

// FormatMetadataMapCLI builds the rendering in the metadataMapper template.
func FormatMetadataMapCLI(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	err := json.Unmarshal(data, &stats)
	if err != nil {
		return b.String(), err
	}
	renderStatusTemplate(b, "/metadatamapper.tmpl", stats)
	return b.String(), nil
}

func renderChecksStats(w io.Writer, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats interface{}, onlyCheck string) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["pyLoaderStats"] = pyLoaderStats
	checkStats["pythonInit"] = pythonInit
	checkStats["AutoConfigStats"] = autoConfigStats
	checkStats["CheckSchedulerStats"] = checkSchedulerStats
	checkStats["OnlyCheck"] = onlyCheck
	checkStats["CheckMetadata"] = inventoriesStats
	renderStatusTemplate(w, "/collector.tmpl", checkStats)
}

func renderCheckStats(data []byte, checkName string) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats) //nolint:errcheck
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	pythonInit := stats["pythonInit"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	inventoriesStats := stats["inventories"]
	renderChecksStats(b, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats, checkName)

	return b.String(), nil
}

func renderComplianceChecksStats(w io.Writer, runnerStats /*, checkSchedulerStats*/ interface{} /*, onlyCheck string*/) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	renderStatusTemplate(w, "/compliance.tmpl", checkStats)
}

func renderStatusTemplate(w io.Writer, templateName string, stats interface{}) {
	tmpl, tmplErr := Asset(templateName)
	if tmplErr != nil {
		fmt.Println(tmplErr)
		return
	}
	t := template.Must(template.New(templateName).Funcs(fmap).Parse(string(tmpl)))
	err := t.Execute(w, stats)
	if err != nil {
		fmt.Println(err)
	}
}
