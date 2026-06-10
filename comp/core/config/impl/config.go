// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configimpl provides the config component implementation.
package configimpl

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v2"

	configdef "github.com/DataDog/datadog-agent/comp/core/config/def"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	pkgconfigmodel.Config

	// warnings are the warnings generated during setup
	warnings *pkgconfigmodel.Warnings
}

// Requires declares the input types to the config component constructor.
type Requires struct {
	Params        configdef.Params
	Secret        secrets.Component
	DelegatedAuth delegatedauth.Component
}

// Provides defines the output of the config component.
type Provides struct {
	Comp          configdef.Component
	FlareProvider flaretypes.Provider
}

// NewServerlessConfig initializes a config component from the given config file.
// TODO: serverless must be eventually migrated to fx, this workaround will then become obsolete.
func NewServerlessConfig(path string) (configdef.Component, error) {
	options := []func(*configdef.Params){configdef.WithConfigName("serverless")}

	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		options = append(options, configdef.WithConfFilePath(path))
	}

	r := Requires{
		Params:        configdef.NewParams(path, options...),
		Secret:        &secretnooptypes.SecretNoop{},
		DelegatedAuth: &delegatedauthnooptypes.DelegatedAuthNoop{},
	}
	return newConfig(r)
}

// NewComponent creates a new config component.
func NewComponent(deps Requires) (Provides, error) {
	c, err := newConfig(deps)
	if err != nil {
		return Provides{}, err
	}
	return Provides{
		Comp:          c,
		FlareProvider: flaretypes.NewProvider(c.fillFlare),
	}, nil
}

func newConfig(deps Requires) (*cfg, error) {
	config := pkgconfigsetup.GlobalConfigBuilder()
	warnings := &pkgconfigmodel.Warnings{}

	err := setupConfig(config, deps.Secret, deps.DelegatedAuth, deps.Params)
	returnErrFct := func(e error) (*cfg, error) {
		if e != nil && deps.Params.GetIgnoreErrors() {
			warnings.Errors = []error{e}
			e = nil
		}
		return &cfg{Config: config, warnings: warnings}, e
	}

	if err != nil {
		return returnErrFct(err)
	}

	if deps.Params.GetConfigLoadSecurityAgent() {
		if err := pkgconfigsetup.Merge(deps.Params.GetSecurityAgentConfigFilePaths(), config); err != nil {
			return returnErrFct(err)
		}
	}

	return &cfg{Config: config, warnings: warnings}, nil
}

// NewCfgFromPkgConfig creates a cfg component from an existing pkgconfigmodel.Config.
// This is used in tests and mock implementations.
func NewCfgFromPkgConfig(pkgCfg pkgconfigmodel.Config) configdef.Component {
	return &cfg{Config: pkgCfg}
}

func (c *cfg) Warnings() *pkgconfigmodel.Warnings {
	return c.warnings
}

func (c *cfg) StartTime() time.Time {
	return c.Config.StartTime()
}

// fillFlare add the Configuration files to flares.
func (c *cfg) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	if mainConfpath := c.ConfigFileUsed(); mainConfpath != "" {
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml")) //nolint:errcheck

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml")) //nolint:errcheck

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "security-agent.yaml")) //nolint:errcheck

		// use best effort to include application_monitoring.yaml to the flare
		// application_monitoring.yaml is a file that lets customers configure Datadog SDKs at the level of the host
		fb.CopyFileTo(filepath.Join(confDir, "application_monitoring.yaml"), filepath.Join("etc", "application_monitoring.yaml")) //nolint:errcheck
	}

	for _, path := range c.ExtraConfigFilesUsed() {
		fb.CopyFileTo(path, filepath.Join("etc/extra_conf/", path)) //nolint:errcheck
	}

	yamlData, err := yaml.Marshal(c.AllSettingsWithoutSecrets())
	if err != nil {
		return err
	}
	return fb.AddFile("runtime_config_dump.yaml", yamlData)
}
