// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package installer

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// EnsurePackagesLayout ensures /opt/datadog-packages directories and symlinks are set up for remote updates.
func (i *installerImpl) EnsurePackagesLayout(ctx context.Context) error {
	// Create directories (idempotent - will only create if they don't exist)
	directories := file.Directories{
		{Path: "/opt/datadog-packages/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/run/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/run", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/tmp", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	if err := directories.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create symlinks only if they don't already exist
	agentVersion := version.AgentPackageVersion

	// Link /opt/datadog-agent -> /opt/datadog-packages/run/datadog-agent/{version}
	versionPath := "/opt/datadog-packages/run/datadog-agent/" + agentVersion
	if _, err := os.Lstat(versionPath); os.IsNotExist(err) {
		if err := file.EnsureSymlink(ctx, "/opt/datadog-agent", versionPath); err != nil {
			return fmt.Errorf("failed to create symlink from /opt/datadog-agent: %w", err)
		}
	}

	// Link /opt/datadog-packages/run/datadog-agent/{version} -> /opt/datadog-packages/datadog-agent/stable
	stablePath := "/opt/datadog-packages/datadog-agent/stable"
	if _, err := os.Lstat(stablePath); os.IsNotExist(err) {
		if err := file.EnsureSymlink(ctx, versionPath, stablePath); err != nil {
			return fmt.Errorf("failed to create stable symlink: %w", err)
		}
	}

	// Link /opt/datadog-packages/datadog-agent/stable -> /opt/datadog-packages/datadog-agent/experiment
	experimentPath := "/opt/datadog-packages/datadog-agent/experiment"
	if _, err := os.Lstat(experimentPath); os.IsNotExist(err) {
		if err := file.EnsureSymlink(ctx, stablePath, experimentPath); err != nil {
			return fmt.Errorf("failed to create experiment symlink: %w", err)
		}
	}

	return nil
}
