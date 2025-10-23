// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime"
	"strings"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// setupConfig loads additional configuration data from yaml files, fleet policies, and command-line options
func setupConfig(config pkgconfigmodel.BuildableConfig, secretComp secrets.Component, p Params) error {
	confFilePath := p.ConfFilePath
	configName := p.configName
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
		return err
	}

	// load the configuration
	err := pkgconfigsetup.LoadDatadog(config, secretComp, pkgconfigsetup.SystemProbe().GetEnvVars())

	if err != nil && (!errors.Is(err, pkgconfigmodel.ErrConfigFileNotFound) || confFilePath != "") {
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
		return err
	}

	// Load the remote configuration
	if p.FleetPoliciesDirPath == "" {
		p.FleetPoliciesDirPath = config.GetString("fleet_policies_dir")
	}
	if p.FleetPoliciesDirPath != "" {
		// Main config file
		err := config.MergeFleetPolicy(path.Join(p.FleetPoliciesDirPath, "datadog.yaml"))
		if err != nil {
			return err
		}
		if p.configLoadSecurityAgent {
			err := config.MergeFleetPolicy(path.Join(p.FleetPoliciesDirPath, "security-agent.yaml"))
			if err != nil {
				return err
			}
		}
	}

	for k, v := range p.cliOverride {
		config.Set(k, v, pkgconfigmodel.SourceCLI)
	}

	return nil
}

// GetInstallPath returns the install path for the agent
func GetInstallPath() string {
	return pkgconfigsetup.InstallPath
}
