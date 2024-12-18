// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrapper provides the installer bootstrapper component.
package bootstrapper

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

// Bootstrap bootstraps the installer and uses it to install the default packages.
func Bootstrap(ctx context.Context, env *env.Env) error {
	version := "latest"
	if env.DefaultPackagesVersionOverride[bootstrap.InstallerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[bootstrap.InstallerPackage]
	}
	installerURL := oci.PackageURL(env, bootstrap.InstallerPackage, version)
	err := bootstrap.Install(ctx, env, installerURL)
	if err != nil {
		return fmt.Errorf("failed to bootstrap the installer: %w", err)
	}
	return exec.NewInstallerExec(env, paths.StableInstallerPath).Setup(ctx)
}
