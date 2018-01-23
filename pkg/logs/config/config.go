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

// private configuration properties
var (
	logsSources *LogSources
)

// GetLogsSources returns the list of logs sources
func GetLogsSources() *LogSources {
	return logsSources
}

// Build initializes logs-agent configuration
func Build() error {
	sources, err := buildLogSources(LogsAgent.GetString("confd_path"))
	if err != nil {
		return err
	}
	logsSources = sources
	return nil
}
