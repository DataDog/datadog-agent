// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

import (
	"log"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/viper"
)

// MainConfig is the name of the main config file, while we haven't merged in dd agent
const MainConfig = "datadog"

// LogsAgent is the global configuration object
var LogsAgent = ddconfig.Datadog

// BuildLogsAgentConfig initializes the LogsAgent config and sets default values
func BuildLogsAgentConfig(ddconfigPath, ddconfdPath string) error {
	return buildMainConfig(LogsAgent, ddconfigPath, ddconfdPath)
}

func buildMainConfig(config *viper.Viper, ddconfigPath, ddconfdPath string) error {
	config.SetConfigFile(ddconfigPath)

	// default values
	config.SetDefault("log_open_files_limit", DefaultTailingLimit)

	err := config.ReadInConfig()
	if err != nil {
		return err
	}

	// For hostname, use value from config if set and non empty,
	// or fallback on agent6's logic
	if config.GetString("hostname") == "" {
		hostname, err := util.GetHostname()
		if err != nil {
			log.Println(err)
			hostname = "unknown"
		}
		config.Set("hostname", hostname)
	}

	err = BuildLogsAgentIntegrationsConfigs(ddconfdPath)
	if err != nil {
		return err
	}
	return nil
}
