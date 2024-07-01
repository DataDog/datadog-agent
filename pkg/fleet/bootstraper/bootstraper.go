// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstraper provides the installer bootstraper component.
package bootstraper

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

const (
	installerPackage = "datadog-installer"
)

// Bootstrap bootstraps the installer and uses it to install the default packages.
func Bootstrap(ctx context.Context, env *env.Env) error {
	version := "latest"
	if env.DefaultPackagesVersionOverride[installerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[installerPackage]
	}
	installerURL := oci.PackageURL(env, installerPackage, version)
	err := bootstrap.Install(ctx, env, installerURL)
	if err != nil {
		return fmt.Errorf("failed to bootstrap the installer: %w", err)
	}

	cmd := exec.NewInstallerExec(env, exec.StableInstallerPath)
	defaultPackages, err := cmd.DefaultPackages(ctx)
	if err != nil {
		return fmt.Errorf("failed to get default packages: %w", err)
	}
	for _, url := range defaultPackages {
		err = cmd.Install(ctx, url, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to install package %s: %v\n", url, err)
		}
	}
	return nil
}
