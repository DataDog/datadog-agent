// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/viper"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) error {
	_, err := SetupConfigWithWarnings(confFilePath, "")
	return err
}

// SetupConfigWithWarnings fires up the configuration system and returns warnings if any.
func SetupConfigWithWarnings(confFilePath, configName string) (*config.Warnings, error) {
	return setupConfig(config.Datadog, "datadog.yaml", confFilePath, configName, false, true, config.SystemProbe.GetEnvVars())
}

// SetupConfigWithoutSecrets fires up the configuration system without secrets support
func SetupConfigWithoutSecrets(confFilePath string, configName string) error {
	_, err := setupConfig(config.Datadog, "datadog.yaml", confFilePath, configName, true, true, config.SystemProbe.GetEnvVars())
	return err
}

func setupConfig(cfg config.Config, origin string, confFilePath string, configName string, withoutSecrets bool, failOnMissingFile bool, additionalKnownEnvVars []string) (*config.Warnings, error) {
	if configName != "" {
		cfg.SetConfigName(configName)
	}
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		cfg.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			cfg.SetConfigFile(confFilePath)
		}
	}
	cfg.AddConfigPath(DefaultConfPath)
	// load the configuration
	warnings, err := config.LoadDatadogCustom(cfg, origin, !withoutSecrets)
	// If `!failOnMissingFile`, do not issue an error if we cannot find the default config file.
	var e viper.ConfigFileNotFoundError
	if err != nil && (failOnMissingFile || !errors.As(err, &e) || confFilePath != "") {
		// special-case permission-denied with a clearer error message
		if errors.Is(err, fs.ErrPermission) {
			if runtime.GOOS == "windows" {
				err = fmt.Errorf(`cannot access the Datadog config file (%w); try running the command in an Administrator shell"`, err)
			} else {
				err = fmt.Errorf("cannot access the Datadog config file (%w); try running the command under the same user as the Datadog Agent", err)
			}
		} else {
			err = fmt.Errorf("unable to load Datadog config file: %w", err)
		}
		return warnings, err
	}
	return warnings, nil
}

// SelectedCheckMatcherBuilder returns a function that returns true if the number of configs found for the
// check name is more or equal to min instances
func SelectedCheckMatcherBuilder(checkNames []string, minInstances uint) func(configs []integration.Config) bool {
	return func(configs []integration.Config) bool {
		var matchedConfigsCount uint
		for _, cfg := range configs {
			for _, name := range checkNames {
				if cfg.Name == name {
					matchedConfigsCount++
				}
			}
		}
		return matchedConfigsCount >= minInstances
	}
}

// SetupInternalProfiling is a common helper to configure runtime settings for internal profiling.
func SetupInternalProfiling(cfg config.ConfigReader, configPrefix string) {
	if v := cfg.GetInt(configPrefix + "internal_profiling.block_profile_rate"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_block_profile_rate", v); err != nil {
			log.Errorf("Error setting block profile rate: %v", err)
		}
	}

	if v := cfg.GetInt(configPrefix + "internal_profiling.mutex_profile_fraction"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_mutex_profile_fraction", v); err != nil {
			log.Errorf("Error mutex profile fraction: %v", err)
		}
	}

	if cfg.GetBool(configPrefix + "internal_profiling.enabled") {
		err := settings.SetRuntimeSetting("internal_profiling", true)
		if err != nil {
			log.Errorf("Error starting profiler: %v", err)
		}
	}
}
