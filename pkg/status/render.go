// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/render"
)

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
	render.Template(b, "header.tmpl", stats)
	render.ChecksStats(b, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats, "")
	renderJMXFetchStatus(b, jmxStats)
	render.Template(b, "forwarder.tmpl", forwarderStats)
	render.Template(b, "logsagent.tmpl", logsStats)
	render.Template(b, "dogstatsd.tmpl", aggregatorStats)
	if config.Datadog.GetBool("cluster_agent.enabled") {
		render.Template(b, "clusteragent.tmpl", dcaStats)
	}

	return b.String(), nil
}

func renderCheckStats(data []byte, checkName string) (string, error) {
	var b = new(bytes.Buffer)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	runnerStats := stats["runnerStats"]
	pyLoaderStats := stats["pyLoaderStats"]
	autoConfigStats := stats["autoConfigStats"]
	checkSchedulerStats := stats["checkSchedulerStats"]
	render.ChecksStats(b, runnerStats, pyLoaderStats, autoConfigStats, checkSchedulerStats, checkName)

	return b.String(), nil
}

func renderJMXFetchStatus(w io.Writer, jmxStats interface{}) {
	stats := make(map[string]interface{})
	stats["JMXStatus"] = jmxStats
	render.Template(w, "jmxfetch.tmpl", stats)
}
