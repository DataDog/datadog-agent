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
	"path/filepath"
	"runtime"
	"strings"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// setupConfig loads additional configuration data from yaml files, fleet policies, and command-line options
func setupConfig(config pkgconfigmodel.BuildableConfig, secretComp secrets.Component, delegatedAuthComp delegatedauth.Component, p Params) error {
	confFilePath := p.ConfFilePath
	configName := p.configName
	defaultConfPath := p.defaultConfPath

	if configName != "" {
		config.SetConfigName(configName)
	}

	// set the paths where a config file is expected
	if confFilePath != "" {
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
	err := pkgconfigsetup.LoadDatadog(config, secretComp, delegatedAuthComp, pkgconfigsetup.SystemProbe().GetEnvVars())

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

	// If -c points to a directory (not a specific .yaml file), and the user has not explicitly
	// set confd_path or additional_checksd, derive them from that directory. This ensures that
	// an agent started with -c /etc/datadog-agent-exp reads integrations from
	// /etc/datadog-agent-exp/conf.d instead of the hardcoded /etc/datadog-agent/conf.d default.
	if confFilePath != "" && !strings.HasSuffix(confFilePath, ".yaml") && !strings.HasSuffix(confFilePath, ".yml") {
		if config.GetSource("confd_path") == pkgconfigmodel.SourceDefault {
			config.Set("confd_path", filepath.Join(confFilePath, "conf.d"), pkgconfigmodel.SourceCLI)
		}
		if config.GetSource("additional_checksd") == pkgconfigmodel.SourceDefault {
			config.Set("additional_checksd", filepath.Join(confFilePath, "checks.d"), pkgconfigmodel.SourceCLI)
		}
	}

	return nil
}

// GetInstallPath returns the install path for the agent
func GetInstallPath() string {
	return pkgconfigsetup.InstallPath
}
