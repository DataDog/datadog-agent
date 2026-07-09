// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	parProcmgrConfigFileName = "datadog-agent-action.yaml"
	parBinaryRelPath         = "bin/agent/privateactionrunner.exe"
)

func parProcmgrConfigRelPath() string {
	return filepath.ToSlash(filepath.Join("processes.d", parProcmgrConfigFileName))
}

// WritePARProcmgrConfig writes datadog-agent-action.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WritePARProcmgrConfig(installRootResolved string) error {
	installRoot, err := os.OpenRoot(installRootResolved)
	if err != nil {
		return fmt.Errorf("open install root: %w", err)
	}
	defer installRoot.Close()

	parExe := filepath.Join(installRootResolved, filepath.FromSlash(parBinaryRelPath))
	if _, err := installRoot.Stat(parBinaryRelPath); err != nil {
		log.Debugf("PAR processes.d: skip write (privateactionrunner.exe stat %s: %v)", parExe, err)
		if errors.Is(err, os.ErrNotExist) {
			return RemovePARProcmgrConfig(installRootResolved)
		}
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		log.Debugf("PAR processes.d: cannot write, DatadogProgramFilesDir is empty (installRoot=%s)", installRootResolved)
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for PAR")
	}
	installPFRoot, err := os.OpenRoot(installPF)
	if err != nil {
		return fmt.Errorf("open program files root: %w", err)
	}
	defer installPFRoot.Close()

	processesDir := filepath.Join(installPF, "processes.d")
	if err := installPFRoot.MkdirAll("processes.d", 0o755); err != nil {
		log.Debugf("PAR processes.d: mkdir %s: %v", processesDir, err)
		return fmt.Errorf("create processes.d: %w", err)
	}

	installRootRepl := filepath.ToSlash(filepath.Clean(installRootResolved))
	etcRootRepl := filepath.ToSlash(filepath.Clean(paths.DatadogDataDir))
	fleetPolicies := paths.FleetPoliciesDirForManagedProcess()
	fleetPoliciesRepl := filepath.ToSlash(filepath.Clean(fleetPolicies))
	log.Debugf("PAR processes.d: template replace __PAR_INSTALL_ROOT__=%q __PAR_ETC_ROOT__=%q __PAR_FLEET_POLICIES_DIR__=%q",
		installRootRepl, etcRootRepl, fleetPoliciesRepl)

	config := embedded.PARWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__PAR_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__PAR_ETC_ROOT__", etcRootRepl)
	config = strings.ReplaceAll(config, "__PAR_FLEET_POLICIES_DIR__", fleetPoliciesRepl)

	configPath := filepath.Join(processesDir, parProcmgrConfigFileName)
	log.Debugf("PAR processes.d: writing %q", configPath)
	if err := installPFRoot.WriteFile(parProcmgrConfigRelPath(), []byte(config), 0o644); err != nil {
		return err
	}
	return nil
}

// RemovePARProcmgrConfig removes the PAR processes.d YAML from the install layout and from
// legacy package-relative processes.d.
func RemovePARProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		installPFRoot, err := os.OpenRoot(installPF)
		if err != nil {
			return fmt.Errorf("open program files root: %w", err)
		}
		defer installPFRoot.Close()

		if err := removePARProcmgrConfigAtRoot(
			installPFRoot,
			filepath.Join(installPF, "processes.d", parProcmgrConfigFileName),
		); err != nil {
			return err
		}
	} else {
		log.Debugf("PAR processes.d: remove skip primary (DatadogProgramFilesDir is empty)")
	}

	packageRoot, err := os.OpenRoot(packageRootResolved)
	if err != nil {
		return fmt.Errorf("open package root: %w", err)
	}
	defer packageRoot.Close()

	legacy := filepath.Join(packageRootResolved, "processes.d", parProcmgrConfigFileName)
	log.Debugf("PAR processes.d: remove legacy %q", legacy)
	if err := removePARProcmgrConfigAtRoot(packageRoot, legacy); err != nil {
		return err
	}

	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		removeEmptyProcessesDir(installPF)
	}
	return nil
}

func removePARProcmgrConfigAtRoot(root *os.Root, absPathForLog string) error {
	log.Debugf("PAR processes.d: remove %q", absPathForLog)
	if err := root.Remove(parProcmgrConfigRelPath()); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("PAR processes.d: remove %q: not present", absPathForLog)
			return nil
		}
		log.Debugf("PAR processes.d: remove %q: %v", absPathForLog, err)
		return err
	}
	return nil
}
