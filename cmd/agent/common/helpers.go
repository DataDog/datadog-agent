// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/viper"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePaths []string) error {
	_, err := SetupConfigWithWarnings(confFilePaths, "")
	return err
}

// SetupConfigWithWarnings fires up the configuration system and returns warnings if any.
func SetupConfigWithWarnings(confFilePaths []string, configName string) (*config.Warnings, error) {
	return setupConfig(confFilePaths, configName, false, true)
}

// SetupConfigWithoutSecrets fires up the configuration system without secrets support
func SetupConfigWithoutSecrets(confFilePaths []string, configName string) error {
	_, err := setupConfig(confFilePaths, configName, true, true)
	return err
}

// SetupConfigIfExist fires up the configuration system but
// doesn't raise an error if the configuration file is the default one
// and it doesn't exist.
func SetupConfigIfExist(confFilePaths []string) error {
	_, err := setupConfig(confFilePaths, "", false, false)
	return err
}

func setupConfig(confFilePaths []string, configName string, withoutSecrets bool, failOnMissingFile bool) (*config.Warnings, error) {
	if len(confFilePaths) == 0 {
		confFilePaths = append(confFilePaths, DefaultConfPath)
	}

	// load the configuration
	var err error
	var warnings *config.Warnings

	if withoutSecrets {
		warnings, err = config.LoadWithoutSecret(configName, confFilePaths, failOnMissingFile)
	} else {
		warnings, err = config.Load(configName, confFilePaths, failOnMissingFile)
	}
	// If `!failOnMissingFile`, do not issue an error if we cannot find the default config file.
	var e viper.ConfigFileNotFoundError
	if err != nil && (failOnMissingFile || !errors.As(err, &e) || len(confFilePaths) > 1 || confFilePaths[0] != DefaultConfPath) {
		return warnings, fmt.Errorf("unable to load Datadog config files: %w", err)
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
