// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package datadogagent implements the datadog agent install methods
package datadogagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/selinux"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	agentPackage       = "datadog-agent"
	agentSymlink       = "/usr/bin/datadog-agent"
	installerSymlink   = "/usr/bin/datadog-installer"
	legacyAgentSymlink = "/opt/datadog-agent"

	// stablePath is the path to the stable agent installation
	stablePath = filepath.Join(paths.PackagesPath, agentPackage, "stable")
	// experimentPath is the path to the experimental agent installation
	experimentPath = filepath.Join(paths.PackagesPath, agentPackage, "experiment")

	// agentDirectories are the directories that the agent needs to function
	agentDirectories = file.Directories{
		{Path: "/etc/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/var/log/datadog", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	// agentConfigPermissions are the ownerships and modes that are enforced on the agent configuration files
	agentConfigPermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "managed", Owner: "root", Group: "root", Recursive: true},
		{Path: "inject", Owner: "root", Group: "root", Recursive: true},
		{Path: "compliance.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "runtime-security.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "system-probe.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "system-probe.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
	}

	// agentPackagePermissions are the ownerships and modes that are enforced on the agent package files
	agentPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "embedded/bin/system-probe", Owner: "root", Group: "root"},
		{Path: "embedded/bin/security-agent", Owner: "root", Group: "root"},
		{Path: "embedded/share/system-probe/ebpf", Owner: "root", Group: "root", Recursive: true},
		{Path: "embedded/share/system-probe/java", Owner: "root", Group: "root", Recursive: true},
	}
)

// PostInstall performs post-installation steps for the agent
func PostInstall(ctx context.Context, installPath string, caller string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "post_install_agent")
	defer func() {
		span.Finish(err)
	}()

	// 1. Ensure the dd-agent user and group exist
	userHomePath := installPath
	if installPath == stablePath || installPath == experimentPath {
		userHomePath = "/opt/datadog-packages"
	}
	if err = user.EnsureAgentUserAndGroup(ctx, userHomePath); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// 2. Ensures the installer is present in the agent package
	installerPath := filepath.Join(installPath, "embedded", "bin", "installer")
	if _, err := os.Stat(installerPath); os.IsNotExist(err) {
		err = installerCopy(installerPath)
		if err != nil {
			return fmt.Errorf("failed to copy installer: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check installer: %w", err)
	}

	// 3. Ensure config/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure(installPath); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}

	// 4. Create symlinks
	if err = file.EnsureSymlink(filepath.Join(installPath, "bin/agent/agent"), agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	if installPath == stablePath {
		if err = file.EnsureSymlink(installPath, legacyAgentSymlink); err != nil {
			return fmt.Errorf("failed to create symlink: %v", err)
		}
	}
	if err = file.EnsureSymlinkIfNotExists(filepath.Join(installPath, "embedded/bin/installer"), installerSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// 5. Set up SELinux permissions
	if err = selinux.SetAgentPermissions("/etc/datadog-agent", installPath); err != nil {
		log.Warnf("failed to set SELinux permissions: %v", err)
	}

	// 6. Handle install info
	if err = installinfo.WriteInstallInfo(caller); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}

	// 7. Call post.py for integration persistence. Allowed to fail.
	// XXX: We should port this to Go
	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/post.py"), installPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed to run integration persistence in post.py: %s\n", err.Error())
		}
	}

	return nil
}

func installerCopy(path string) error {
	// Copy the current executable to the installer path
	// This is temporary and will be removed after next release
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}

	sourceFile, err := os.Open(currentExecutable)
	if err != nil {
		return fmt.Errorf("failed to open current executable: %w", err)
	}
	defer sourceFile.Close()

	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create installer directory: %w", err)
	}
	destinationFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	err = destinationFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("failed to set permissions on destination file: %w", err)
	}
	return nil
}
