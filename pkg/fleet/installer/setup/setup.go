// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup contains the different setup scenarios
package setup

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/djm"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

const (
	// FlavorDatabricks is the flavor for the Data Jobs Monitoring databricks setup.
	FlavorDatabricks = "databricks"
)

// Setup installs Datadog.
func Setup(ctx context.Context, env *env.Env, flavor string) error {
	switch flavor {
	case FlavorDatabricks:
		if err := packages.SetupInstaller(ctx); err != nil {
			return fmt.Errorf("failed to setup installer: %w", err)
		}
		if err := djm.SetupDatabricks(ctx, env); err != nil {
			return fmt.Errorf("failed to setup Databricks: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown setup flavor %s", flavor)
	}
}

// Agent7InstallScript is the setup used by the agent7 install script.
func Agent7InstallScript(ctx context.Context, env *env.Env) error {
	cmd := exec.NewInstallerExec(env, paths.StableInstallerPath)
	defaultPackages, err := cmd.DefaultPackages(ctx)
	if err != nil {
		return fmt.Errorf("failed to get default packages: %w", err)
	}
	for _, url := range defaultPackages {
		err = cmd.Install(ctx, url, nil)
		if err != nil {
			return fmt.Errorf("failed to install package %s: %w", url, err)
		}
	}
	return nil
}
