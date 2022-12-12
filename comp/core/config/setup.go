// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/DataDog/viper"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// setupConfig is copied from cmd/agent/common/helpers.go.
func setupConfig(
	confFilePath string,
	configName string,
	withoutSecrets bool,
	failOnMissingFile bool,
	defaultConfPath string) (*config.Warnings, error) {
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
	if defaultConfPath != "" {
		config.Datadog.AddConfigPath(defaultConfPath)
	}

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

// MergeConfigurationFiles reads an array of configuration filenames and attempts to merge them. The userDefined value is used to specify that configurationFilesArray contains filenames defined on the command line
// TODO(paulcacheux): change this a component method once all security-agent commands have been converted to fx
func MergeConfigurationFiles(configName string, configurationFilesArray []string, userDefined bool) (*config.Warnings, error) {
	// we'll search for a config file named `datadog.yaml`
	config.Datadog.SetConfigName(configName)

	// Track if a configuration file was loaded
	loadedConfiguration := false

	var warnings *config.Warnings

	// Load and merge configuration files
	for _, configurationFilename := range configurationFilesArray {
		if _, err := os.Stat(configurationFilename); err != nil {
			if userDefined {
				fmt.Printf("Warning: unable to access %s\n", configurationFilename)
			}
			continue
		}
		if !loadedConfiguration {
			w, err := setupConfig(configurationFilename, "", false, true, "")
			if err != nil {
				if userDefined {
					fmt.Printf("Warning: unable to open %s\n", configurationFilename)
				}
				continue
			}
			warnings = w
			loadedConfiguration = true
		} else {
			file, err := os.Open(configurationFilename)
			if err != nil {
				if userDefined {
					fmt.Printf("Warning: unable to open %s\n", configurationFilename)
				}
				continue
			}

			err = config.Datadog.MergeConfig(file)
			if err != nil {
				return warnings, fmt.Errorf("unable to merge a configuration file: %v", err)
			}
		}
	}

	if !loadedConfiguration {
		return warnings, fmt.Errorf("unable to load any configuration file from %s", configurationFilesArray)
	}

	return warnings, nil
}
