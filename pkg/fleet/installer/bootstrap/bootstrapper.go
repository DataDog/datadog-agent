// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// Bootstrap bootstraps the installer and uses it to install the default packages.
func Bootstrap(ctx context.Context, env *env.Env) error {
	version := "latest"
	if env.DefaultPackagesVersionOverride[InstallerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[InstallerPackage]
	}
	installerURL := oci.PackageURL(env, InstallerPackage, version)
	err := Install(ctx, env, installerURL)
	if err != nil {
		return fmt.Errorf("failed to bootstrap the installer: %w", err)
	}
	return exec.NewInstallerExec(env, paths.StableInstallerPath).Setup(ctx)
}
