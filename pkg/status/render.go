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
	autoConfigStats := stats["autoConfigStats"]
	aggregatorStats := stats["aggregatorStats"]
	jmxStats := stats["JMXStatus"]
	logsStats := stats["logsStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(b, stats)
	renderChecksStats(b, runnerStats, autoConfigStats, "")
	renderJMXFetchStatus(b, jmxStats)
	renderForwarderStatus(b, forwarderStats)
	renderLogsStatus(b, logsStats)
	renderDogstatsdStatus(b, aggregatorStats)

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
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(b, stats)
	renderChecksStats(b, runnerStats, autoConfigStats, "")
	renderForwarderStatus(b, forwarderStats)

	return b.String(), nil
}

// FormatServiceMapCLI builds the rendering in the servicemapper template.
func FormatServiceMapCLI(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	err := json.Unmarshal(data, &stats)
	if err != nil {
		return b.String(), nil
	}
	renderServiceMapper(b, stats)

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

func renderChecksStats(w io.Writer, runnerStats interface{}, autoConfigStats interface{}, onlyCheck string) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["AutoConfigStats"] = autoConfigStats
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
	autoConfigStats := stats["autoConfigStats"]
	renderChecksStats(b, runnerStats, autoConfigStats, checkName)

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

func renderServiceMapper(w io.Writer, serviceMapperStats interface{}) {
	t := template.Must(template.New("servicemapper.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "servicemapper.tmpl")))
	err := t.Execute(w, serviceMapperStats)
	if err != nil {
		fmt.Println(err)
	}
}
