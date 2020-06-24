// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package standalone

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetupCLI sets up the shared utilities for a standalone CLI command:
// - config, with defaults to avoid conflicting with an agent process running in parallel
// - logger
// and returns the log level resolved from cliLogLevel and defaultLogLevel
func SetupCLI(loggerName config.LoggerName, confFilePath string, cliLogLevel string, defaultLogLevel string) (string, error) {
	var resolvedLogLevel string

	if cliLogLevel != "" {
		// Honour the deprecated --log-level argument
		overrides := make(map[string]interface{})
		overrides["log_level"] = cliLogLevel
		config.AddOverrides(overrides)
		resolvedLogLevel = cliLogLevel
	} else {
		resolvedLogLevel = config.GetEnv("DD_LOG_LEVEL", defaultLogLevel)
	}

	overrides := make(map[string]interface{})
	overrides["cmd_port"] = 0 // let the OS assign an available port for the HTTP server
	config.AddOverrides(overrides)

	err := common.SetupConfig(confFilePath)
	if err != nil {
		return resolvedLogLevel, fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, resolvedLogLevel, "", "", false, true, false)
	if err != nil {
		return resolvedLogLevel, fmt.Errorf("unable to set up logger: %v", err)
	}

	return resolvedLogLevel, nil
}
