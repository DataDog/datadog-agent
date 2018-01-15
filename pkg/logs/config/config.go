// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// LogsAgent is the global configuration object
var LogsAgent = ddconfig.Datadog

// private configuration properties
var (
	logsSources []*IntegrationConfigLogSource
)

// GetLogsSources returns the list of logs sources
func GetLogsSources() []*IntegrationConfigLogSource {
	return logsSources
}

// Build initializes logs-agent configuration
func Build() error {
	sources, sourcesToTrack, err := buildLogsSources(LogsAgent.GetString("confd_path"))
	if err != nil {
		return err
	}
	logsSources = sources
	status.Initialize(sourcesToTrack)
	return nil
}
