// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup contains the different setup scenarios
package setup

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/djm"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

type flavor struct {
	path string // path is used to print the path to the setup script for users.
	run  func(*common.Setup) error
}

var flavors = map[string]flavor{
	"databricks": {path: "djm/databricks.go", run: djm.SetupDatabricks},
}

// Setup installs Datadog.
func Setup(ctx context.Context, env *env.Env, flavor string) error {
	f, ok := flavors[flavor]
	if !ok {
		return fmt.Errorf("unknown flavor %s", flavor)
	}
	s, err := common.NewSetup(ctx, env, flavor, f.path, os.Stdout)
	if err != nil {
		return err
	}
	err = f.run(s)
	if err != nil {
		return err
	}
	return s.Run()
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
