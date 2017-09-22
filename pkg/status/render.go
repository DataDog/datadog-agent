// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"path/filepath"

	"github.com/kardianos/osext"
)

var (
	here, _        = osext.ExecutableFolder()
	fmap           template.FuncMap
	templateFolder string
)

func init() {
	templateFolder = filepath.Join(here, "dist", "templates")
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
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(b, stats)
	renderChecksStats(b, runnerStats, autoConfigStats, "")
	renderJMXFetchStatus(b, jmxStats)
	renderForwarderStatus(b, forwarderStats)
	renderAggregatorStatus(b, aggregatorStats)

	return b.String(), nil
}

func renderHeader(w io.Writer, stats map[string]interface{}) {
	t := template.Must(template.New("header.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "header.tmpl")))
	err := t.Execute(w, stats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderAggregatorStatus(w io.Writer, aggregatorStats interface{}) {
	t := template.Must(template.New("aggregator.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "aggregator.tmpl")))
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
