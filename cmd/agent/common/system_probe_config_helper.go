// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build process,!windows

package common

import (
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	proc_config "github.com/DataDog/datadog-agent/pkg/process/config"
)

const sysProbeConfigFile = "system-probe.yaml"

// SetupSystemProbeConfig reads the system-probe.yaml into the global config object
func SetupSystemProbeConfig(sysProbeConfFilePath string) error {
	// Open the system-probe.yaml file if it's in a custom location
	if sysProbeConfFilePath != "" {
		// If file is not set directly assume we need to add /system-probe.yaml
		if !strings.HasSuffix(sysProbeConfFilePath, ".yaml") {
			sysProbeConfFilePath = path.Join(sysProbeConfFilePath, sysProbeConfigFile)
		}
	} else {
		// Assume it is in the default location if nothing is passed in
		sysProbeConfFilePath = path.Join(DefaultConfPath, sysProbeConfigFile)
	}

	file, err := os.Open(sysProbeConfFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Merge config with an IO reader since this lets us merge the configs without changing
	// the config file set with viper
	if err := config.Datadog.MergeConfig(file); err != nil {
		return err
	}

	// The full path to the location of the unix socket where connections will be accessed
	// This is not necessarily set in the system-probe.yaml, so set it manually if it is not
	if !config.Datadog.IsSet("system_probe_config.sysprobe_socket") {
		config.Datadog.Set("system_probe_config.sysprobe_socket", proc_config.GetSocketPath())
	}

	// Load the env vars last to overwrite values
	proc_config.LoadSysProbeEnvVariables()
	return nil
}
