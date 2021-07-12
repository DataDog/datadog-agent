// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/viper"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) error {
	_, err := SetupConfigWithWarnings(confFilePath, "")
	return err
}

// SetupConfigWithWarnings fires up the configuration system and returns warnings if any.
func SetupConfigWithWarnings(confFilePath, configName string) (*config.Warnings, error) {
	return setupConfig(confFilePath, configName, false, true)
}

// SetupConfigWithoutSecrets fires up the configuration system without secrets support
func SetupConfigWithoutSecrets(confFilePath string, configName string) error {
	_, err := setupConfig(confFilePath, configName, true, true)
	return err
}

// SetupConfigIfExist fires up the configuration system but
// doesn't raise an error if the configuration file is the default one
// and it doesn't exist.
func SetupConfigIfExist(confFilePath string) error {
	_, err := setupConfig(confFilePath, "", false, false)
	return err
}

func setupConfig(confFilePath string, configName string, withoutSecrets bool, failOnMissingFile bool) (*config.Warnings, error) {
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
	var warnings *config.Warnings

	if withoutSecrets {
		warnings, err = config.LoadWithoutSecret()
	} else {
		warnings, err = config.Load()
	}
	// If `!failOnMissingFile`, do not issue an error if we cannot find the default config file.
	var e viper.ConfigFileNotFoundError
	if err != nil && (failOnMissingFile || !errors.As(err, &e) || confFilePath != "") {
		return warnings, fmt.Errorf("unable to load Datadog config file: %w", err)
	}
	return warnings, nil
}
