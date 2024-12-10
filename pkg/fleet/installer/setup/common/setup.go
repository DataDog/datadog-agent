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
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	ErrNoAPIKey = errors.New("no API key provided")
)

// Setup allows setup scripts to define packages and configurations to install.
type Setup struct {
	Span     ddtrace.Span
	Packages Packages
	Config   Config
}

// NewSetup creates a new Setup structure with some default values.
func NewSetup(ctx context.Context, env *env.Env, name string) (*Setup, error) {
	if env.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	span, _ := tracer.StartSpanFromContext(ctx, fmt.Sprintf("setup.%s", name))
	s := &Setup{
		Span: span,
		Config: Config{
			DatadogYAML: DatadogConfig{
				APIKey: env.APIKey,
				Site:   env.Site,
				Env:    os.Getenv("DD_ENV"),
			},
			IntegrationConfigs: make(map[string]IntegrationConfig),
		},
		Packages: Packages{
			install:          make(map[string]string),
			versionOverrides: env.DefaultPackagesVersionOverride,
		},
	}
	return s, nil
}

// Exec installs the packages and writes the configurations
func (s *Setup) Exec(ctx context.Context, installer installer.Installer) (err error) {
	defer func() { s.Span.Finish(tracer.WithError(err)) }()
	err = s.Config.write(configDir)
	if err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}
	return nil
}
