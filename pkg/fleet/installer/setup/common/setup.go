// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common defines the Setup structure that allows setup scripts to define packages and configurations to install.
package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	// ErrNoAPIKey is returned when no API key is provided.
	ErrNoAPIKey = errors.New("no API key provided")
)

// Setup allows setup scripts to define packages and configurations to install.
type Setup struct {
	configDir           string
	installer           installer.Installer
	installerPackageURL string

	Env      *env.Env
	Ctx      context.Context
	Span     ddtrace.Span
	Packages Packages
	Config   Config
}

// NewSetup creates a new Setup structure with some default values.
func NewSetup(ctx context.Context, env *env.Env, name string) (*Setup, error) {
	if env.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	installerPackageURL := fmt.Sprintf("file://%s", filepath.Dir(executablePath))
	installer, err := installer.NewInstaller(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create installer: %w", err)
	}
	span, ctx := tracer.StartSpanFromContext(ctx, fmt.Sprintf("setup.%s", name))
	s := &Setup{
		configDir:           configDir,
		installer:           installer,
		installerPackageURL: installerPackageURL,
		Env:                 env,
		Ctx:                 ctx,
		Span:                span,
		Config: Config{
			DatadogYAML: DatadogConfig{
				APIKey:   env.APIKey,
				Hostname: os.Getenv("DD_HOSTNAME"),
				Site:     env.Site,
				Env:      os.Getenv("DD_ENV"),
			},
			IntegrationConfigs: make(map[string]IntegrationConfig),
		},
		Packages: Packages{
			install: make(map[string]packageWithVersion),
		},
	}
	return s, nil
}

// Run installs the packages and writes the configurations
func (s *Setup) Run() (err error) {
	defer func() { s.Span.Finish(tracer.WithError(err)) }()
	err = writeConfigs(s.Config, s.configDir)
	if err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}
	err = s.installer.Install(s.Ctx, s.installerPackageURL, nil)
	if err != nil {
		return fmt.Errorf("failed to install installer: %w", err)
	}
	packages := resolvePackages(s.Packages)
	for _, p := range packages {
		url := oci.PackageURL(s.Env, p.name, p.version)
		err = s.installer.Install(s.Ctx, url, nil)
		if err != nil {
			return fmt.Errorf("failed to install package %s: %w", url, err)
		}
	}
	return nil
}
