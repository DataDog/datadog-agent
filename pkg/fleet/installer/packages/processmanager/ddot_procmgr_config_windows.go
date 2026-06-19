// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package processmanager implements fleet installer helpers for DDOT and dd-procmgr on Windows.
package processmanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const ddotProcmgrConfigFileName = "datadog-agent-ddot.yaml"

// WriteDDOTProcmgrConfig writes datadog-agent-ddot.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WriteDDOTProcmgrConfig(installRootResolved string) error {
	otelExe := filepath.Join(installRootResolved, "ext", "ddot", "embedded", "bin", "otel-agent.exe")
	if _, err := os.Stat(otelExe); err != nil {
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for DDOT")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		return fmt.Errorf("create processes.d: %w", err)
	}

	fleetPolicies := paths.FleetPoliciesDirForManagedProcess()

	config := embedded.DDOTWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__DDOT_INSTALL_ROOT__", filepath.ToSlash(filepath.Clean(installRootResolved)))
	config = strings.ReplaceAll(config, "__DDOT_ETC_ROOT__", filepath.ToSlash(filepath.Clean(paths.DatadogDataDir)))
	config = strings.ReplaceAll(config, "__DDOT_FLEET_POLICIES_DIR__", filepath.ToSlash(filepath.Clean(fleetPolicies)))

	path := filepath.Join(processesDir, ddotProcmgrConfigFileName)
	return os.WriteFile(path, []byte(config), 0o644)
}

// RemoveDDOTProcmgrConfig removes the DDOT processes.d YAML from the install layout and from
// legacy package-relative processes.d.
func RemoveDDOTProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", ddotProcmgrConfigFileName)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", ddotProcmgrConfigFileName)
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
