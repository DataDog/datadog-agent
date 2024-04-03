// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Reader is a subset of Config that only allows reading of configuration
type Reader = pkgconfigmodel.Reader //nolint:revive

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
	getParams() *Params
	getSecretResolver() secrets.Component
}

type dependencies struct {
	fx.In

	Params Params
	// secrets Component is optional, if not provided, the config will not decrypt secrets
	Secret secrets.Component `optional:"true"`
}

func (d dependencies) getParams() *Params {
	return &d.Params
}

func (d dependencies) getSecretResolver() secrets.Component {
	return d.Secret
}

// NewServerlessConfig initializes a config component from the given config file
// TODO: serverless must be eventually migrated to fx, this workaround will then become obsolete - ts should not be created directly in this fashion.
func NewServerlessConfig(path string) (Component, error) {
	options := []func(*Params){WithConfigName("serverless")}

	_, err := os.Stat(path)
	if os.IsNotExist(err) &&
		(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		options = append(options, WithConfigMissingOK(true))
	} else if !os.IsNotExist(err) {
		options = append(options, WithConfFilePath(path))
	}

	d := dependencies{Params: NewParams(path, options...)}
	return newConfig(d)
}

func newConfig(deps dependencies) (Component, error) {
	config := pkgconfigsetup.Datadog
	warnings, err := setupConfig(config, deps)
	returnErrFct := func(e error) (Component, error) {
		if e != nil && deps.Params.ignoreErrors {
			if warnings == nil {
				warnings = &pkgconfigmodel.Warnings{}
			}
			warnings.Err = e
			e = nil
		}
		return &cfg{Config: config, warnings: warnings}, e
	}

	if err != nil {
		return returnErrFct(err)
	}

	if deps.Params.configLoadSecurityAgent {
		if err := pkgconfigsetup.Merge(deps.Params.securityAgentConfigFilePaths, config); err != nil {
			return returnErrFct(err)
		}
	}

	return &cfg{Config: config, warnings: warnings}, nil
}

func (c *cfg) Warnings() *pkgconfigmodel.Warnings {
	return c.warnings
}
