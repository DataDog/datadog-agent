// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	iexec "github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
)

const (
	// InstallerPackage is the name of the Datadog Installer OCI package
	InstallerPackage = "datadog-installer"
	// AgentPackage is the name of the Datadog Agent OCI package
	AgentPackage = "datadog-agent"
)

// Install self-installs the installer package from the given URL.
func Install(ctx context.Context, env *env.Env, url string) error {
	return install(ctx, env, url, false)
}

// InstallExperiment installs a package from the given URL as an experiment.
// It first tries to grab the installer from a specific layer to start the experiment with,
// and falls back to the current installer if it doesn't exist.
func InstallExperiment(ctx context.Context, env *env.Env, url string) error {
	return install(ctx, env, url, true)
}

// getLocalInstaller returns an installer executor from the current binary
func getLocalInstaller(env *env.Env) (*iexec.InstallerExec, error) {
	installerBin, err := iexec.GetExecutable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	return iexec.NewInstallerExec(env, installerBin), nil
}
