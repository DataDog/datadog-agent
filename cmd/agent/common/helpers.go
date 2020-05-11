// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	proc_config "github.com/DataDog/datadog-agent/pkg/process/config"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) error {
	return setupConfig(confFilePath, "", false)
}

// SetupConfigWithoutSecrets fires up the configuration system without secrets support
func SetupConfigWithoutSecrets(confFilePath string, configName string) error {
	return setupConfig(confFilePath, configName, true)
}

func setupConfig(confFilePath string, configName string, withoutSecrets bool) error {
	if configName != "" {
		config.Datadog.SetConfigName(configName)
	}
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			config.Datadog.SetConfigFile(confFilePath)
		}
	}
	config.Datadog.AddConfigPath(DefaultConfPath)
	// load the configuration
	var err error
	if withoutSecrets {
		err = config.LoadWithoutSecret()
	} else {
		err = config.Load()
	}
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}
	return nil
}

// SetupSystemProbeConfig adds the system-probe.yaml file to the config object using an io.Reader so we don't overwrite the datadog.yaml
func SetupSystemProbeConfig(sysProbeConfFilePath string) error {
	var (
		file *os.File
		err  error
	)

	// Open the system-probe.yaml file if it's in a custom location
	if len(sysProbeConfFilePath) != 0 {
		// If config path is passed in, assume we should use it
		if strings.HasSuffix(sysProbeConfFilePath, ".yaml") {
			// Open the file directly if they pass the full path
			file, err = os.Open(sysProbeConfFilePath)
		} else {
			file, err = os.Open(sysProbeConfFilePath + "/system-probe.yaml")
		}
	} else {
		// Assume it is in the default location if nothing is passed in
		file, err = os.Open(DefaultConfPath + "/system-probe.yaml")
	}

	defer file.Close()
	if err != nil {
		return err
	}

	// Merge config with an IO reader since this lets us merge the configs without changing
	// the config file set with the viper
	if err := config.Datadog.MergeConfig(file); err != nil {
		return err
	}

	// The full path to the location of the unix socket where connections will be accessed
	// This is not necessarily set in the system-probe.yaml, so set it manually if it is not
	if !config.Datadog.IsSet("system_probe_config.sysprobe_socket") {
		config.Datadog.Set("system_probe_config.sysprobe_socket", proc_config.GetSocketPath())
	}

	// Load the env vars last to overwrite what might be set in the config file
	proc_config.LoadSysProbeEnvVariables()

	log.Info(config.Datadog.GetBool("system_probe_config.enabled"))
	log.Info(config.Datadog.GetBool("system_probe_config.bpf_debug"))
	log.Info(config.Datadog.Get("ac_include"))
	log.Info(config.Datadog.Get("system_probe_config.sysprobe_socket"))
	log.Info(config.Datadog.Get("apm_config.enabled"))
	log.Info(config.Datadog.ConfigFileUsed())

	return nil
}
