// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"strings"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"
)

// LogsAgent is the global configuration object
var LogsAgent = ddconfig.Datadog

// private configuration properties
var (
	hostname    string
	logsSources []*IntegrationConfigLogSource
)

// GetHostname returns logs-agent hostname
func GetHostname() string {
	return hostname
}

// GetLogsSources returns the list of logs sources
func GetLogsSources() []*IntegrationConfigLogSource {
	return logsSources
}

// Build initializes logs-agent configuration
func Build() error {
	sources, err := buildLogsSources(LogsAgent.GetString("confd_path"))
	if err != nil {
		return err
	}
	logsSources = sources
	hostname = buildHostname(LogsAgent)
	return nil
}

// buildHostname computes the hostname for logs-agent
func buildHostname(config *viper.Viper) string {
	configHostname := config.GetString("hostname")
	if strings.TrimSpace(configHostname) == "" {
		hostname, err := util.GetHostname()
		if err != nil {
			log.Warn(err)
			hostname = "unknown"
		}
		return hostname
	}
	return configHostname
}
