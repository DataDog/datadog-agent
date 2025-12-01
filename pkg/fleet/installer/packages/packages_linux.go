// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

var (
	// packagesHooks is a map of package names to their hooks
	packagesHooks = map[string]hooks{
		"datadog-agent":      datadogAgentPackage,
		"datadog-apm-inject": apmInjectPackage,
		"datadog-agent-ddot": datadogAgentDDOTPackage,
	}

	// AsyncPreRemoveHooks is called before a package is removed from the disk.
	// It can block the removal of the package files until a condition is met without blocking
	// the rest of the uninstall or upgrade process.
	// Today this is only useful for the dotnet tracer on windows and generally *SHOULD BE AVOIDED*.
	AsyncPreRemoveHooks = map[string]repository.PreRemoveHook{}

	// packageCommands is a map of package names to their command handlers
	packageCommands = map[string]PackageCommandHandler{}
)

// restartServiceImpl implements restart service for Linux
func restartServiceImpl(ctx context.Context, manager string, targetName string) error {
	switch manager {
	case "systemctl":
		cmd := exec.CommandContext(ctx, "systemctl", "restart", targetName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restart service %s: %w", targetName, err)
		}
		return nil
	case "docker":
		cmd := exec.CommandContext(ctx, "docker", "restart", targetName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restart container %s: %w", targetName, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported manager: %s", manager)
	}
}
