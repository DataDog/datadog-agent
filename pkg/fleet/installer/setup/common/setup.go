// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package common defines the Setup structure that allows setup scripts to define packages and configurations to install.
package common

import (
	"context"
	"errors"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

var (
	ErrNoAPIKey = errors.New("no API key provided")
)

// Setup allows setup scripts to define packages and configurations to install.
type Setup struct {
	Packages Packages
	Config   Config
}

// NewSetup creates a new Setup structure with some default values.
func NewSetup(env *env.Env) (*Setup, error) {
	if env.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	s := &Setup{
		Config: Config{
			DatadogYAML: DatadogConfig{
				APIKey: env.APIKey,
				Site:   env.Site,
				Env:    os.Getenv("DD_ENV"),
			},
			IntegrationConfigs: make(map[string]IntegrationConfig),
		},
	}
	return s, nil
}

// Run installs the packages and writes the configurations
func (i *Setup) Run(ctx context.Context) error {

	return nil
}
