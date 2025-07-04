// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// team: agent-configuration

package configimpl

import (
	"os"
	"path/filepath"
	"strings"

	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type Requires struct {
	Params config.Params
	Secret option.Option[secrets.Component]
}

type Provides struct {
	Comp          config.Component
	FlareProvider flaretypes.Provider
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	pkgconfigmodel.Config

	// warnings are the warnings generated during setup
	warnings *pkgconfigmodel.Warnings
}

// configDependencies is an interface that mimics the fx-oriented dependencies struct
// TODO: (components) investigate whether this interface is worth keeping, otherwise delete it and just use dependencies
type configDependencies interface {
	getParams() *config.Params
	getSecretResolver() (secrets.Component, bool)
}

func (d Requires) getParams() *config.Params {
	return &d.Params
}

func (d Requires) getSecretResolver() (secrets.Component, bool) {
	return d.Secret.Get()
}

// NewServerlessConfig initializes a config component from the given config file
// TODO: serverless must be eventually migrated to fx, this workaround will then become obsolete - ts should not be created directly in this fashion.
func NewServerlessConfig(path string) (config.Component, error) {
	options := []func(*config.Params){config.WithConfigName("serverless")}

	_, err := os.Stat(path)
	if os.IsNotExist(err) &&
		(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		options = append(options, config.WithConfigMissingOK(true))
	} else if !os.IsNotExist(err) {
		options = append(options, config.WithConfFilePath(path))
	}

	d := Requires{Params: config.NewParams(path, options...)}
	return newConfig(d)
}

func NewComponent(deps Requires) (Provides, error) {
	c, err := newConfig(deps)
	return Provides{
		Comp:          c,
		FlareProvider: flaretypes.NewProvider(c.fillFlare),
	}, err
}

func newConfig(deps configDependencies) (*cfg, error) {
	configObj := pkgconfigsetup.Datadog()
	warnings, err := setupConfig(configObj, deps)
	returnErrFct := func(e error) (*cfg, error) {
		if e != nil && deps.getParams().IgnoreErrors {
			if warnings == nil {
				warnings = &pkgconfigmodel.Warnings{}
			}
			warnings.Errors = []error{e}
			e = nil
		}
		return &cfg{Config: configObj, warnings: warnings}, e
	}

	if err != nil {
		return returnErrFct(err)
	}

	if deps.getParams().ConfigLoadSecurityAgent {
		if err := pkgconfigsetup.Merge(deps.getParams().SecurityAgentConfigFilePaths, configObj); err != nil {
			return returnErrFct(err)
		}
	}

	return &cfg{Config: configObj, warnings: warnings}, nil
}

func (c *cfg) Warnings() *pkgconfigmodel.Warnings {
	return c.warnings
}

// fillFlare add the Configuration files to flares.
func (c *cfg) fillFlare(fb flaretypes.FlareBuilder) error {
	if mainConfpath := c.ConfigFileUsed(); mainConfpath != "" {
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml")) //nolint:errcheck

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml")) //nolint:errcheck

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "security-agent.yaml")) //nolint:errcheck
	}

	for _, path := range c.ExtraConfigFilesUsed() {
		fb.CopyFileTo(path, filepath.Join("etc/extra_conf/", path)) //nolint:errcheck
	}

	return nil
}
