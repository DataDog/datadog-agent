// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// LogsAgent is the global configuration object
var LogsAgent = config.Datadog

// Build returns logs-agent sources
func Build() *LogSources {
	return buildLogSources(LogsAgent.GetString("confd_path"))
}

// buildLogSources returns all the logs sources computed from logs configuration files and environment variables
func buildLogSources(ddconfdPath string) *LogSources {
	var sources []*LogSource

	// append sources from all logs config files
	fileSources := buildLogSourcesFromDirectory(ddconfdPath)
	sources = append(sources, fileSources...)

	return NewLogSources(sources)
}
