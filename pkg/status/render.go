// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"text/template"

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
	forwarderStats := stats["forwarderStats"]
	if forwarderStatsMap, ok := forwarderStats.(map[string]interface{}); ok {
		forwarderStatsMap["config"] = stats["config"]
	} else {
		log.Warn("The Forwarder status format is invalid. Some parts of the `Forwarder` section may be missing.")
	}
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	pythonInit := stats["pythonInit"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
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
	inventoriesStats := stats["inventories"]
	systemProbeStats := stats["systemProbeStats"]
	processAgentStatus := stats["processAgentStatus"]
	snmpTrapsStats := stats["snmpTrapsStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	headerFunc := func() error { return RenderStatusTemplate(b, "/header.tmpl", stats) }
	checkStatsFunc := func() error {
		return renderChecksStats(b, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats, "")
	}
	jmxFetchFunc := func() error { return RenderStatusTemplate(b, "/jmxfetch.tmpl", stats) }
	forwarderFunc := func() error { return RenderStatusTemplate(b, "/forwarder.tmpl", forwarderStats) }
	endpointsFunc := func() error { return RenderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos) }
	logsAgentFunc := func() error { return RenderStatusTemplate(b, "/logsagent.tmpl", logsStats) }
	systemProbeFunc := func() error {
		if systemProbeStats != nil {
			return RenderStatusTemplate(b, "/systemprobe.tmpl", systemProbeStats)
		}
		return nil
	}
	processAgentFunc := func() error { return RenderStatusTemplate(b, "/process-agent.tmpl", processAgentStatus) }
	traceAgentFunc := func() error { return RenderStatusTemplate(b, "/trace-agent.tmpl", stats["apmStats"]) }
	aggregatorFunc := func() error { return RenderStatusTemplate(b, "/aggregator.tmpl", aggregatorStats) }
	dogstatsdFunc := func() error { return RenderStatusTemplate(b, "/dogstatsd.tmpl", dogstatsdStats) }
	clusterAgentFunc := func() error {
		if config.Datadog.GetBool("cluster_agent.enabled") || config.Datadog.GetBool("cluster_checks.enabled") {
			return RenderStatusTemplate(b, "/clusteragent.tmpl", dcaStats)
		}
		return nil
	}
	snmpTrapFunc := func() error {
		if traps.IsEnabled() {
			return RenderStatusTemplate(b, "/snmp-traps.tmpl", snmpTrapsStats)
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
		return RenderStatusTemplate(b, "/remoteconfig.tmpl", stats)
	}
	otlpFunc := func() error {
		if otlp.IsDisplayed() {
			return RenderStatusTemplate(b, "/otlp.tmpl", stats)
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
			clusterAgentFunc, snmpTrapFunc, autodiscoveryFunc, remoteConfigFunc, otlpFunc}
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
	runnerStats := stats["runnerStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	endpointsInfos := stats["endpointsInfos"]
	logsStats := stats["logsStats"]
	orchestratorStats := stats["orchestrator"]
	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	var errs []error
	if err := RenderStatusTemplate(b, "/header.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderChecksStats(b, runnerStats, nil, nil, autoConfigStats, checkSchedulerStats, nil, ""); err != nil {
		errs = append(errs, err)
	}
	if err := RenderStatusTemplate(b, "/forwarder.tmpl", forwarderStats); err != nil {
		errs = append(errs, err)
	}
	if err := RenderStatusTemplate(b, "/endpoints.tmpl", endpointsInfos); err != nil {
		errs = append(errs, err)
	}
	if config.Datadog.GetBool("compliance_config.enabled") {
		if err := RenderStatusTemplate(b, "/logsagent.tmpl", logsStats); err != nil {
			errs = append(errs, err)
		}
	}
	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		if err := RenderStatusTemplate(b, "/orchestrator.tmpl", orchestratorStats); err != nil {
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
	if err := RenderStatusTemplate(b, "/custommetricsprovider.tmpl", stats); err != nil {
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
	if err := RenderStatusTemplate(b, "/header.tmpl", stats); err != nil {
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
	if err := RenderStatusTemplate(b, "/process-agent.tmpl", stats); err != nil {
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
	if err := RenderStatusTemplate(b, "/metadatamapper.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return "", err
	}
	return b.String(), nil
}

func renderChecksStats(w io.Writer, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats interface{}, onlyCheck string) error {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["pyLoaderStats"] = pyLoaderStats
	checkStats["pythonInit"] = pythonInit
	checkStats["AutoConfigStats"] = autoConfigStats
	checkStats["CheckSchedulerStats"] = checkSchedulerStats
	checkStats["OnlyCheck"] = onlyCheck
	checkStats["CheckMetadata"] = inventoriesStats
	return RenderStatusTemplate(w, "/collector.tmpl", checkStats)
}

func renderCheckStats(data []byte, checkName string) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	pythonInit := stats["pythonInit"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	inventoriesStats := stats["inventories"]
	var b = new(bytes.Buffer)
	var errs []error
	if err := renderChecksStats(b, runnerStats, pyLoaderStats, pythonInit, autoConfigStats, checkSchedulerStats, inventoriesStats, checkName); err != nil {
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
	return RenderStatusTemplate(w, "/compliance.tmpl", checkStats)
}

func renderRuntimeSecurityStats(w io.Writer, runtimeSecurityStatus interface{}) error {
	status := make(map[string]interface{})
	status["RuntimeSecurityStatus"] = runtimeSecurityStatus
	return RenderStatusTemplate(w, "/runtimesecurity.tmpl", status)
}

func renderAutodiscoveryStats(w io.Writer, adEnabledFeatures interface{}, adConfigErrors interface{}, filterErrors interface{}) error {
	autodiscoveryStats := make(map[string]interface{})
	autodiscoveryStats["adEnabledFeatures"] = adEnabledFeatures
	autodiscoveryStats["adConfigErrors"] = adConfigErrors
	autodiscoveryStats["filterErrors"] = filterErrors
	return RenderStatusTemplate(w, "/autodiscovery.tmpl", autodiscoveryStats)
}

//go:embed templates
var templatesFS embed.FS

func RenderStatusTemplate(w io.Writer, templateName string, stats interface{}) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", templateName))
	if tmplErr != nil {
		return tmplErr
	}
	t := template.Must(template.New(templateName).Funcs(fmap).Parse(string(tmpl)))
	return t.Execute(w, stats)
}

func renderErrors(w io.Writer, errs []error) error {
	if len(errs) > 0 {
		return RenderStatusTemplate(w, "/rendererrors.tmpl", errs)
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
