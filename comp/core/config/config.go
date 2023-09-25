// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/conf"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	conf.Config

	// warnings are the warnings generated during setup
	warnings *conf.Warnings
}

type configDependencies interface {
	getParams() *Params
}

type dependencies struct {
	fx.In

	Params Params
}

func (d dependencies) getParams() *Params {
	return &d.Params
}

// TODO: serverless must be eventually migrated to fx, this workaround
//
//	will then become obsolete - ts should not be created
//	directly in this fashion.
func NewServerlessConfig(path string, config conf.Config, origin string, additionalKnownEnvVars []string) (Component, error) {
	options := []func(*Params){WithConfigName("serverless"), WithConfigLoadSecrets(true)}

	_, err := os.Stat(path)
	if os.IsNotExist(err) &&
		(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		options = append(options, WithConfigMissingOK(true))
	} else if !os.IsNotExist(err) {
		options = append(options, WithConfFilePath(path))
	}

	d := dependencies{Params: NewParams(path, options...)}
	return newConfig(d, config, origin, additionalKnownEnvVars)
}

func newConfig(deps dependencies, config conf.Config, origin string, additionalKnownEnvVars []string) (Component, error) {
	warnings, err := setupConfig(deps, config, origin, additionalKnownEnvVars)
	returnErrFct := func(e error) (Component, error) {
		if e != nil && deps.Params.ignoreErrors {
			if warnings == nil {
				warnings = &conf.Warnings{}
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
		if err := conf.Merge(deps.Params.securityAgentConfigFilePaths, config); err != nil {
			return returnErrFct(err)
		}
	}

	return &cfg{Config: config, warnings: warnings}, nil
}

func (c *cfg) Warnings() *conf.Warnings {
	return c.warnings
}

func (c *cfg) Object() conf.ConfigReader {
	return c.Config
}
