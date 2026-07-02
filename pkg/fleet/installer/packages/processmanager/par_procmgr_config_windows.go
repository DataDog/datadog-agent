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

const parProcmgrConfigFileName = "datadog-agent-action.yaml"

// WritePARProcmgrConfig writes datadog-agent-action.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WritePARProcmgrConfig(installRootResolved string) error {
	parExe := filepath.Join(installRootResolved, "bin", "agent", "privateactionrunner.exe")
	if _, err := os.Stat(parExe); err != nil {
		log.Debugf("PAR processes.d: skip write (privateactionrunner.exe stat %s: %v)", parExe, err)
		if os.IsNotExist(err) {
			return RemovePARProcmgrConfig(installRootResolved)
		}
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		log.Debugf("PAR processes.d: cannot write, DatadogProgramFilesDir is empty (installRoot=%s)", installRootResolved)
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for PAR")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
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

	path := filepath.Join(processesDir, parProcmgrConfigFileName)
	log.Debugf("PAR processes.d: writing %q", path)
	return os.WriteFile(path, []byte(config), 0o644)
}

// RemovePARProcmgrConfig removes the PAR processes.d YAML from the install layout and from
// legacy package-relative processes.d.
func RemovePARProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", parProcmgrConfigFileName)
		log.Debugf("PAR processes.d: remove %q", p)
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				log.Debugf("PAR processes.d: remove %q: not present", p)
			} else {
				log.Debugf("PAR processes.d: remove %q: %v", p, err)
				return err
			}
		}
	} else {
		log.Debugf("PAR processes.d: remove skip primary (DatadogProgramFilesDir is empty)")
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", parProcmgrConfigFileName)
	log.Debugf("PAR processes.d: remove legacy %q", legacy)
	if err := os.Remove(legacy); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("PAR processes.d: remove legacy %q: not present", legacy)
		} else {
			log.Debugf("PAR processes.d: remove legacy %q: %v", legacy, err)
			return err
		}
	}
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		removeEmptyProcessesDir(installPF)
	}
	return nil
}
