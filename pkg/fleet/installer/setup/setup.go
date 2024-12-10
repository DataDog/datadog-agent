// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup contains the different setup scenarios
package setup

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/agent"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/djm"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

const (
	// FlavorManagedAgent is the flavor for the managed agent setup.
	FlavorManagedAgent = "managed-agent"
	// FlavorDatabricks is the flavor for the Data Jobs Monitoring databricks setup.
	FlavorDatabricks = "databricks"
)

// Setup installs Datadog.
func Setup(ctx context.Context, env *env.Env, flavor string) error {
	i, err := installer.NewInstaller(env)
	if err != nil {
		return err
	}
	s, err := common.NewSetup(ctx, env, flavor)
	if err != nil {
		return err
	}
	switch flavor {
	case FlavorManagedAgent:
		agent.Setup(s)
	case FlavorDatabricks:
		djm.SetupDatabricks(s)
	default:
		return fmt.Errorf("unknown setup flavor %s", flavor)
	}
	return s.Exec(ctx, i)
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
