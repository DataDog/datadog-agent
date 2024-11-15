// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

const (
	// InstallerPackage is the name of the Datadog Installer OCI package
	InstallerPackage = "datadog-installer"
	installerBinPath = "bin/installer/installer"
)

// Install self-installs the installer package from the given URL.
func Install(ctx context.Context, env *env.Env, url string) error {
	return install(ctx, env, url, false)
}

// InstallExperiment self-installs the installer package from the given URL as an experiment.
func InstallExperiment(ctx context.Context, env *env.Env, url string) error {
	return install(ctx, env, url, true)
}
