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
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/selinux"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentSymlink       = "/usr/bin/datadog-agent"
	installerSymlink   = "/usr/bin/datadog-installer"
	legacyAgentSymlink = "/opt/datadog-agent"
)

var (
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

// setupFilesystem sets up the filesystem for the agent installation
func setupFilesystem(ctx context.Context, installPath string, caller string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "setup_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// 1. Ensure the dd-agent user and group exist
	userHomePath := installPath
	if installPath == StablePath || installPath == ExperimentPath {
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
	if installPath == StablePath {
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
	return nil
}

// removeFilesystem cleans the filesystem
// All operations are allowed to fail
func removeFilesystem(ctx context.Context, installPath string) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_filesystem")
	defer func() {
		span.Finish(nil)
	}()

	// Remove run dir
	os.RemoveAll(filepath.Join(installPath, "run"))
	// Remove FIPS module
	os.Remove(filepath.Join(installPath, "embedded", "ssl", "fipsmodule.cnf"))
	// Remove any file related to reinstalling non-core integrations (see python-scripts/packages.py for the names)
	os.Remove(filepath.Join(installPath, ".pre_python_installed_packages.txt"))
	os.Remove(filepath.Join(installPath, ".post_python_installed_packages.txt"))
	os.Remove(filepath.Join(installPath, ".diff_python_installed_packages.txt"))
	// Remove install info
	installinfo.RemoveInstallInfo()
	// Remove symlinks
	os.Remove(agentSymlink)
	os.Remove(legacyAgentSymlink)
	if target, err := os.Readlink(installerSymlink); err == nil && strings.HasPrefix(target, installPath) {
		os.Remove(installerSymlink)
	}
}

// installerCopy copies the current executable to the installer path
func installerCopy(path string) error {
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

// removeCompiledPythonFiles removes compiled Python files (.pyc, .pyo) and __pycache__ directories
func removeCompiledPythonFiles(installPath string) {
	// Remove files in in "{installPath}/embedded/.py_compiled_files.txt"
	if _, err := os.Stat(filepath.Join(installPath, "embedded/.py_compiled_files.txt")); err == nil {
		compiledFiles, err := os.ReadFile(filepath.Join(installPath, "embedded/.py_compiled_files.txt"))
		if err != nil {
			fmt.Printf("failed to read compiled files list: %s\n", err.Error())
		} else {
			for _, file := range strings.Split(string(compiledFiles), "\n") {
				if strings.HasPrefix(file, installPath) {
					if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
						fmt.Printf("failed to remove compiled file %s: %s\n", file, err.Error())
					}
				}
			}
		}
	}
	// Remove files in {installPath}/bin/agent/dist
	err := filepath.Walk(filepath.Join(installPath, "bin", "agent", "dist"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("failed to remove compiled files: %s\n", err.Error())
	}
	// Remove files in {installPath}/python-scripts
	err = filepath.Walk(filepath.Join(installPath, "python-scripts"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("failed to remove compiled files: %s\n", err.Error())
	}
}
