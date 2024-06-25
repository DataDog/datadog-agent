// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"

	"github.com/DataDog/viper"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// setupConfig is copied from cmd/agent/common/helpers.go.
func setupConfig(config pkgconfigmodel.Config, deps configDependencies) (*pkgconfigmodel.Warnings, error) {
	p := deps.getParams()

	confFilePath := p.ConfFilePath
	configName := p.configName
	failOnMissingFile := !p.configMissingOK
	defaultConfPath := p.defaultConfPath

	if configName != "" {
		config.SetConfigName(configName)
	}

	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") || strings.HasSuffix(confFilePath, ".yml") {
			config.SetConfigFile(confFilePath)
		}
	}
	if defaultConfPath != "" {
		config.AddConfigPath(defaultConfPath)
	}

	// load extra config file paths
	if err := config.AddExtraConfigPaths(p.ExtraConfFilePath); err != nil {
		return nil, err
	}

	// load the configuration
	var err error
	var warnings *pkgconfigmodel.Warnings
	if resolver, ok := deps.getSecretResolver(); ok {
		warnings, err = pkgconfigsetup.LoadWithSecret(config, resolver, pkgconfigsetup.SystemProbe.GetEnvVars())
	} else {
		warnings, err = pkgconfigsetup.LoadWithoutSecret(config, pkgconfigsetup.SystemProbe.GetEnvVars())
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
