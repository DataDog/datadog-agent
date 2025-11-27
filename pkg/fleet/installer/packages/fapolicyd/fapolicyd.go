// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package fapolicyd offers an interface to set agent's fapolicyd permissions.
package fapolicyd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const datadogFapolicydPriority = 39

var fapolicydProfilePath = filepath.Join("/etc/fapolicyd/rules.d/", fmt.Sprintf("%d-datadog.rules", datadogFapolicydPriority))

// fapolicydPermissions defines the permissions to be set for the agent in fapolicyd.
//
// 1. Allow /opt/datadog-packages/** to be executed by any user or process.
// 2. Allow all to execute binaries in /opt/datadog-packages/**.
// 3. Allow /opt/datadog-packages/** to open any file in the filesystem (shared libraries).
var fapolicydPermissions = fmt.Sprintf(`allow perm=execute dir=%[1]s : all
allow perm=execute all : dir=%[1]s
allow perm=open dir=%[1]s : all`, paths.PackagesPath)

// SetAgentPermissions sets the fapolicyd permissions for the agent if the OS requires it.
//
// Fortunately for us the default fapolicyd lets users / bash scripts call any binary on disk if they are root,
// so we can execute this in the first installer binary to be called.
// For the sake of simplicity we'll assume a default fapolicyd configuration, in terms of priority.
func SetAgentPermissions(ctx context.Context) (err error) {
	if !isFapolicydSupported() || !isFagenrulesSupported() {
		return nil
	}

	span, _ := telemetry.StartSpanFromContext(ctx, "setup_fapolicyd")
	defer func() { span.Finish(err) }()

	if err = os.WriteFile(fapolicydProfilePath, []byte(fapolicydPermissions), 0644); err != nil {
		return err
	}

	if err = telemetry.CommandContext(ctx, "fagenrules", "--load").Run(); err != nil {
		return fmt.Errorf("failed to load fagenrules: %w", err)
	}

	return nil
}

// isFapolicydSupported checks if fapolicyd is installed on the host
func isFapolicydSupported() bool {
	_, err := exec.LookPath("fapolicyd")
	return err == nil
}

// isFagenrulesSupported checks if fagenrules is installed on the host
func isFagenrulesSupported() bool {
	_, err := exec.LookPath("fagenrules")
	return err == nil
}
