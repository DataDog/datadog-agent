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

	htmlTemplate "html/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var fmap = status.TextFmap()
var htmlfmap = status.HTMLFmap()

// FormatStatus takes a json bytestring and prints out the formatted statuspage
func FormatStatus(data []byte) (string, error) {
	stats, renderError, err := unmarshalStatus(data)
	if renderError != "" || err != nil {
		return renderError, err
	}
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	headerFunc := func() error { return ParseTemplate(b, "/header.tmpl", stats) }
	checkStatsFunc := func() error {
		return ParseTemplate(b, "/collector.tmpl", stats)
	}
	jmxFetchFunc := func() error { return ParseTemplate(b, "/jmxfetch.tmpl", stats) }
	forwarderFunc := func() error { return ParseTemplate(b, "/forwarder.tmpl", stats) }
	endpointsFunc := func() error { return ParseTemplate(b, "/endpoints.tmpl", stats) }
	logsAgentFunc := func() error { return ParseTemplate(b, "/logsagent.tmpl", stats) }
	systemProbeFunc := func() error { return ParseTemplate(b, "/systemprobe.tmpl", stats) }
	processAgentFunc := func() error { return ParseTemplate(b, "/process-agent.tmpl", stats) }
	traceAgentFunc := func() error { return ParseTemplate(b, "/trace-agent.tmpl", stats) }
	aggregatorFunc := func() error { return ParseTemplate(b, "/aggregator.tmpl", stats) }
	dogstatsdFunc := func() error { return ParseTemplate(b, "/dogstatsd.tmpl", stats) }
	clusterAgentFunc := func() error { return ParseTemplate(b, "/clusteragent.tmpl", stats) }
	snmpTrapFunc := func() error { return ParseTemplate(b, "/snmp-traps.tmpl", stats) }
	netflowFunc := func() error { return ParseTemplate(b, "/netflow.tmpl", stats) }
	autodiscoveryFunc := func() error { return ParseTemplate(b, "/autodiscovery.tmpl", stats) }
	remoteConfigFunc := func() error { return ParseTemplate(b, "/remoteconfig.tmpl", stats) }
	otlpFunc := func() error { return ParseTemplate(b, "/otlp.tmpl", stats) }

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

	// We nil these keys because we do not want to display that information in the collector template
	stats["pyLoaderStats"] = nil
	stats["pythonInit"] = nil
	stats["inventories"] = nil

	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	var errs []error
	if err := ParseTemplate(b, "/header.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/collector.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/forwarder.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/endpoints.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/logsagent.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/autodiscovery.tmpl", stats); err != nil {
		errs = append(errs, err)
	}

	if err := ParseTemplate(b, "/orchestrator.tmpl", stats); err != nil {
		errs = append(errs, err)
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
	if err := ParseTemplate(b, "/custommetricsprovider.tmpl", stats); err != nil {
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

	title := fmt.Sprintf("Datadog Security Agent (v%s)", stats["version"])
	stats["title"] = title

	var b = new(bytes.Buffer)
	var errs []error
	if err := ParseTemplate(b, "/header.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/runtimesecurity.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := ParseTemplate(b, "/compliance.tmpl", stats); err != nil {
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
	if err := ParseTemplate(b, "/process-agent.tmpl", stats); err != nil {
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
	if err := ParseTemplate(b, "/metadatamapper.tmpl", stats); err != nil {
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
	if err := ParseTemplate(b, "/collector.tmpl", stats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return "", err
	}

	return b.String(), nil
}

// FormatAgentTelemetry takes a json bytestring and prints out the formatted agent telemetry template.
func FormatAgentTelemetry(stats map[string]interface{}, renderTmplSuffix string) ([]byte, error) {
	telemetrystats := make(map[string]interface{})

	// Pluck the relevant parts of the status JSON
	if rs, ok := stats["runnerStats"]; ok {
		telemetrystats["RunnerStats"] = rs
	}
	if dsd, ok := stats["dogstatsdStats"]; ok {
		telemetrystats["DogStatsdStats"] = dsd
	}

	var b = new(bytes.Buffer)
	var errs []error
	telemetryTemplate := fmt.Sprintf("/agent-telemetry-%s.tmpl", renderTmplSuffix)
	if err := ParseTemplate(b, telemetryTemplate, telemetrystats); err != nil {
		errs = append(errs, err)
	}
	if err := renderErrors(b, errs); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

//go:embed templates
var templatesFS embed.FS

// ParseTemplate renders the text template with the data provided
func ParseTemplate(w io.Writer, templateName string, stats interface{}) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", templateName))
	if tmplErr != nil {
		return tmplErr
	}
	t, err := template.New(templateName).Funcs(fmap).Parse(string(tmpl))
	if err != nil {
		return err
	}
	return t.Execute(w, stats)
}

// ParseHTMLTemplate the HTML template with the data provided
func ParseHTMLTemplate(w io.Writer, templateName string, stats interface{}) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", templateName))
	if tmplErr != nil {
		return tmplErr
	}
	t, err := htmlTemplate.New(templateName).Funcs(htmlfmap).Parse(string(tmpl))
	if err != nil {
		return err
	}
	return t.Execute(w, stats)
}

func renderErrors(w io.Writer, errs []error) error {
	if len(errs) > 0 {
		return ParseTemplate(w, "/rendererrors.tmpl", errs)
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
