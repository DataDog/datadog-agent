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
	"golang.org/x/sys/windows/registry"
)

const (
	processProcmgrConfigFileName = "datadog-agent-process.yaml"

	processProcmgrConfigWrittenThisInstallRegistryKey = "ProcessProcmgrConfigWrittenThisInstall"
)

// WriteProcessProcmgrConfig writes datadog-agent-process.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WriteProcessProcmgrConfig(installRootResolved string) error {
	processExe := filepath.Join(installRootResolved, "bin", "agent", "process-agent.exe")
	if _, err := os.Stat(processExe); err != nil {
		log.Debugf("process-agent processes.d: skip write (process-agent.exe stat %s: %v)", processExe, err)
		if os.IsNotExist(err) {
			return RemoveProcessProcmgrConfig(installRootResolved)
		}
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		log.Debugf("process-agent processes.d: cannot write, DatadogProgramFilesDir is empty (installRoot=%s)", installRootResolved)
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for process-agent")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		log.Debugf("process-agent processes.d: mkdir %s: %v", processesDir, err)
		return fmt.Errorf("create processes.d: %w", err)
	}

	installRootRepl := filepath.ToSlash(filepath.Clean(installRootResolved))
	etcRootRepl := filepath.ToSlash(filepath.Clean(paths.DatadogDataDir))
	fleetPolicies := paths.FleetPoliciesDirForManagedProcess()
	fleetPoliciesRepl := filepath.ToSlash(filepath.Clean(fleetPolicies))
	log.Debugf("process-agent processes.d: template replace __PROCESS_INSTALL_ROOT__=%q __PROCESS_ETC_ROOT__=%q __PROCESS_FLEET_POLICIES_DIR__=%q",
		installRootRepl, etcRootRepl, fleetPoliciesRepl)

	config := embedded.ProcessWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__PROCESS_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__PROCESS_ETC_ROOT__", etcRootRepl)
	config = strings.ReplaceAll(config, "__PROCESS_FLEET_POLICIES_DIR__", fleetPoliciesRepl)

	path := filepath.Join(processesDir, processProcmgrConfigFileName)
	_, statErr := os.Stat(path)
	existedBefore := statErr == nil
	log.Debugf("process-agent processes.d: writing %q", path)
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return err
	}
	if !existedBefore {
		if err := setProcessProcmgrConfigWrittenThisInstall(); err != nil {
			log.Warnf("process-agent processes.d: could not mark config written this install: %v", err)
		}
	}
	return nil
}

// RemoveProcessProcmgrConfig removes the process-agent processes.d YAML from the install layout
// and from legacy package-relative processes.d.
func RemoveProcessProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", processProcmgrConfigFileName)
		log.Debugf("process-agent processes.d: remove %q", p)
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				log.Debugf("process-agent processes.d: remove %q: not present", p)
			} else {
				log.Debugf("process-agent processes.d: remove %q: %v", p, err)
				return err
			}
		}
	} else {
		log.Debugf("process-agent processes.d: remove skip primary (DatadogProgramFilesDir is empty)")
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", processProcmgrConfigFileName)
	log.Debugf("process-agent processes.d: remove legacy %q", legacy)
	if err := os.Remove(legacy); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("process-agent processes.d: remove legacy %q: not present", legacy)
		} else {
			log.Debugf("process-agent processes.d: remove legacy %q: %v", legacy, err)
			return err
		}
	}
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		removeEmptyProcessesDir(installPF)
	}
	if err := clearProcessProcmgrConfigWrittenThisInstall(); err != nil {
		log.Debugf("process-agent processes.d: clear install marker: %v", err)
	}
	return nil
}

func setProcessProcmgrConfigWrittenThisInstall() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, datadogAgentRegistryKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(processProcmgrConfigWrittenThisInstallRegistryKey, 1)
}

func clearProcessProcmgrConfigWrittenThisInstall() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, datadogAgentRegistryKey, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(processProcmgrConfigWrittenThisInstallRegistryKey); err == registry.ErrNotExist {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}
