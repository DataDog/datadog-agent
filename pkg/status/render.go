// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

var (
	here, _        = executable.Folder()
	fmap           template.FuncMap
	templateFolder string
)

func init() {
	templateFolder = filepath.Join(common.GetDistPath(), "templates")
}

// FormatStatus takes a json bytestring and prints out the formatted statuspage
func FormatStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	aggregatorStats := stats["aggregatorStats"]
	jmxStats := stats["JMXStatus"]
	logsStats := stats["logsStats"]
	dcaStats := stats["clusterAgentStatus"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(b, stats)
	renderChecksStats(b, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats, "")
	renderJMXFetchStatus(b, jmxStats)
	renderForwarderStatus(b, forwarderStats)
	renderLogsStatus(b, logsStats)
	renderDogstatsdStatus(b, aggregatorStats)
	if config.Datadog.GetBool("cluster_agent.enabled") {
		renderDatadogClusterAgentStatus(b, dcaStats)
	}

	return b.String(), nil
}

// FormatDCAStatus takes a json bytestring and prints out the formatted statuspage
func FormatDCAStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(b, stats)
	renderChecksStats(b, runnerStats, nil, autoConfigStats, checkSchedulerStats, "")
	renderForwarderStatus(b, forwarderStats)

	return b.String(), nil
}

// FormatHPAStatus takes a json bytestring and prints out the formatted statuspage
func FormatHPAStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)
	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	renderHPAStats(b, stats)
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
	renderMetadataMapper(b, stats)

	return b.String(), nil
}

func renderHeader(w io.Writer, stats map[string]interface{}) {
	t := template.Must(template.New("header.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "header.tmpl")))
	err := t.Execute(w, stats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderDogstatsdStatus(w io.Writer, aggregatorStats interface{}) {
	t := template.Must(template.New("dogstatsd.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "dogstatsd.tmpl")))
	err := t.Execute(w, aggregatorStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderForwarderStatus(w io.Writer, forwarderStats interface{}) {
	t := template.Must(template.New("forwarder.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "forwarder.tmpl")))
	err := t.Execute(w, forwarderStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderDatadogClusterAgentStatus(w io.Writer, dcaStats interface{}) {
	t := template.Must(template.New("clusteragent.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "clusteragent.tmpl")))
	err := t.Execute(w, dcaStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderHPAStats(w io.Writer, hpaStats interface{}) {
	t := template.Must(template.New("custommetricsprovider.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "custommetricsprovider.tmpl")))
	err := t.Execute(w, hpaStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderChecksStats(w io.Writer, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats interface{}, onlyCheck string) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["pyLoaderStats"] = pyLoaderStats
	checkStats["AutoConfigStats"] = autoConfigStats
	checkStats["CheckSchedulerStats"] = checkSchedulerStats
	checkStats["OnlyCheck"] = onlyCheck
	t := template.Must(template.New("collector.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "collector.tmpl")))

	err := t.Execute(w, checkStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderCheckStats(data []byte, checkName string) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	renderChecksStats(b, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats, checkName)

	return b.String(), nil
}

func renderJMXFetchStatus(w io.Writer, jmxStats interface{}) {
	stats := make(map[string]interface{})
	stats["JMXStatus"] = jmxStats
	t := template.Must(template.New("jmxfetch.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "jmxfetch.tmpl")))

	err := t.Execute(w, stats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderLogsStatus(w io.Writer, logsStats interface{}) {
	t := template.Must(template.New("logsagent.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "logsagent.tmpl")))
	err := t.Execute(w, logsStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderMetadataMapper(w io.Writer, metadataMapperStats interface{}) {
	t := template.Must(template.New("metadatamapper.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "metadatamapper.tmpl")))
	err := t.Execute(w, metadataMapperStats)
	if err != nil {
		fmt.Println(err)
	}
}
