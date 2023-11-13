// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package render has all the formating options for status output
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"text/template"

	"github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var fmap = Textfmap()

// FormatStatus takes a json bytestring and prints out the formatted statuspage
func FormatStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	aggregatorStats := stats["aggregatorStats"]
	s, err := checkstats.TranslateEventPlatformEventTypes(aggregatorStats)
	if err != nil {
		log.Debugf("failed to translate event platform event types in aggregatorStats: %s", err.Error())
	} else {
		aggregatorStats = s
	}
	dogstatsdStats := stats["dogstatsdStats"]
	logsStats := stats["logsStats"]
	dcaStats := stats["clusterAgentStatus"]
	endpointsInfos := stats["endpointsInfos"]
	systemProbeStats := stats["systemProbeStats"]
	processAgentStatus := stats["processAgentStatus"]
	snmpTrapsStats := stats["snmpTrapsStats"]
	netflowStats := stats["netflowStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	headerFunc := func() error { return renderStatusTemplate(b, "/header.tmpl", stats) }
	checkStatsFunc := func() error {
		return renderStatusTemplate(b, "/collector.tmpl", stats)
	}
	jmxFetchFunc := func() error { return renderStatusTemplate(b, "/jmxfetch.tmpl", stats) }
	forwarderFunc := func() error { return renderStatusTemplate(b, "/forwarder.tmpl", stats) }
	endpointsFunc := func() error { return renderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos) }
	logsAgentFunc := func() error { return renderStatusTemplate(b, "/logsagent.tmpl", logsStats) }
	systemProbeFunc := func() error {
		if systemProbeStats != nil {
			return renderStatusTemplate(b, "/systemprobe.tmpl", systemProbeStats)
		}
		return nil
	}
	processAgentFunc := func() error { return renderStatusTemplate(b, "/process-agent.tmpl", processAgentStatus) }
	traceAgentFunc := func() error { return renderStatusTemplate(b, "/trace-agent.tmpl", stats["apmStats"]) }
	aggregatorFunc := func() error { return renderStatusTemplate(b, "/aggregator.tmpl", aggregatorStats) }
	dogstatsdFunc := func() error { return renderStatusTemplate(b, "/dogstatsd.tmpl", dogstatsdStats) }
	clusterAgentFunc := func() error {
		if config.Datadog.GetBool("cluster_agent.enabled") || config.Datadog.GetBool("cluster_checks.enabled") {
			return renderStatusTemplate(b, "/clusteragent.tmpl", dcaStats)
		}
		return nil
	}
	snmpTrapFunc := func() error {
		if traps.IsEnabled(config.Datadog) {
			return renderStatusTemplate(b, "/snmp-traps.tmpl", snmpTrapsStats)
		}
		return nil
	}

	netflowFunc := func() error {
		if server.IsEnabled() {
			return renderStatusTemplate(b, "/netflow.tmpl", netflowStats)
		}
		return nil
	}

	autodiscoveryFunc := func() error {
		if config.IsContainerized() {
			return renderAutodiscoveryStats(b, stats["adEnabledFeatures"], stats["adConfigErrors"],
				stats["filterErrors"])
		}
		return nil
	}
	remoteConfigFunc := func() error {
		return renderStatusTemplate(b, "/remoteconfig.tmpl", stats)
	}
	otlpFunc := func() error {
		if otlp.IsDisplayed() {
			return renderStatusTemplate(b, "/otlp.tmpl", stats)
		}
		return nil
	}

	var renderFuncs []func() error
	if config.IsCLCRunner() {
		renderFuncs = []func() error{headerFunc, checkStatsFunc, aggregatorFunc, endpointsFunc, clusterAgentFunc,
			autodiscoveryFunc}
	} else {
		renderFuncs = []func() error{headerFunc, checkStatsFunc, jmxFetchFunc, forwarderFunc, endpointsFunc,
			logsAgentFunc, systemProbeFunc, processAgentFunc, traceAgentFunc, aggregatorFunc, dogstatsdFunc,
			clusterAgentFunc, snmpTrapFunc, netflowFunc, autodiscoveryFunc, remoteConfigFunc, otlpFunc}
	}
	var errs []error
	for _, f := range renderFuncs {
		if err := f(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}

	return b.String(), nil
}

// FormatDCAStatus takes a json bytestring and prints out the formatted statuspage
func FormatDCAStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}

	forwarderStats := stats["forwarderStats"]
	// We nil these keys because we do not want to display that information in the collector template
	stats["pyLoaderStats"] = nil
	stats["pythonInit"] = nil
	stats["inventories"] = nil
	endpointsInfos := stats["endpointsInfos"]
	logsStats := stats["logsStats"]
	orchestratorStats := stats["orchestrator"]
	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/header.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderStatusTemplate(b, "/collector.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderStatusTemplate(b, "/forwarder.tmpl", forwarderStats); err != nil {
		errs = append(errs, err)
	}
	if err := renderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos); err != nil {
		errs = append(errs, err)
	}
	if config.Datadog.GetBool("compliance_config.enabled") {
		if err := renderStatusTemplate(b, "/logsagent.tmpl", logsStats); err != nil {
			errs = append(errs, err)
		}
	}
	if err := renderAutodiscoveryStats(b, stats["adEnabledFeatures"], stats["adConfigErrors"], stats["filterErrors"]); err != nil {
		errs = append(errs, err)
	}
	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		if err := renderStatusTemplate(b, "/orchestrator.tmpl", orchestratorStats); err != nil {
			errs = append(errs, err)
		}
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}

	return b.String(), nil
}

// FormatHPAStatus takes a json bytestring and prints out the formatted statuspage
func FormatHPAStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/custommetricsprovider.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}
	return b.String(), nil
}

// FormatSecurityAgentStatus takes a json bytestring and prints out the formatted status for security agent
func FormatSecurityAgentStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	runnerStats := stats["runnerStats"]
	complianceChecks := stats["complianceChecks"]
	complianceStatus := stats["complianceStatus"]
	title := fmt.Sprintf("Datadog Security Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/header.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderRuntimeSecurityStats(b, stats["runtimeSecurityStatus"]); err != nil {
		errs = append(errs, err)
	}
	if err := renderComplianceChecksStats(b, runnerStats, complianceChecks, complianceStatus); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}

	return b.String(), nil
}

// FormatProcessAgentStatus takes a json bytestring and prints out the formatted status for process-agent
func FormatProcessAgentStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/process-agent.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		fmt.Println(err)
	}

	return b.String(), nil
}

// FormatMetadataMapCLI builds the rendering in the metadataMapper template.
func FormatMetadataMapCLI(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/metadatamapper.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return "", err
	}
	return b.String(), nil
}

// FormatCheckStats takes a json bytestring and prints out the formatted collector template.
func FormatCheckStats(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}

	var b = new(bytes.Buffer)
	var errs []error
	if err := renderStatusTemplate(b, "/collector.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return "", err
	}

	return b.String(), nil
}

func renderComplianceChecksStats(w io.Writer, runnerStats interface{}, complianceChecks, complianceStatus interface{}) error {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["ComplianceStatus"] = complianceStatus
	checkStats["ComplianceChecks"] = complianceChecks
	return renderStatusTemplate(w, "/compliance.tmpl", checkStats)
}

func renderRuntimeSecurityStats(w io.Writer, runtimeSecurityStatus interface{}) error {
	status := make(map[string]interface{})
	status["RuntimeSecurityStatus"] = runtimeSecurityStatus
	return renderStatusTemplate(w, "/runtimesecurity.tmpl", status)
}

func renderAutodiscoveryStats(w io.Writer, adEnabledFeatures interface{}, adConfigErrors interface{}, filterErrors interface{}) error {
	autodiscoveryStats := make(map[string]interface{})
	autodiscoveryStats["adEnabledFeatures"] = adEnabledFeatures
	autodiscoveryStats["adConfigErrors"] = adConfigErrors
	autodiscoveryStats["filterErrors"] = filterErrors
	return renderStatusTemplate(w, "/autodiscovery.tmpl", autodiscoveryStats)
}

//go:embed templates
var templatesFS embed.FS

func renderStatusTemplate(w io.Writer, templateName string, stats interface{}) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", templateName))
	if tmplErr != nil {
		return tmplErr
	}
	t := template.Must(template.New(templateName).Funcs(fmap).Parse(string(tmpl)))
	return t.Execute(w, stats)
}

func renderErrors(w io.Writer, errs []error) error {
	if len(errs) > 0 {
		return renderStatusTemplate(w, "/rendererrors.tmpl", errs)
	}
	return nil
}

func unmarshalStatus(data []byte) (stats map[string]interface{}, renderError string, err error) {
	if err := json.Unmarshal(data, &stats); err != nil {
		var b = new(bytes.Buffer)
		if err := renderErrors(b, []error{err}); err != nil {
			return nil, "", err
		}
		return nil, b.String(), nil
	}
	return stats, "", nil
}
