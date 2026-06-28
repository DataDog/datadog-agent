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

const adpProcmgrConfigFileName = "datadog-agent-data-plane.yaml"

// WriteADPProcmgrConfig writes datadog-agent-data-plane.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WriteADPProcmgrConfig(installRootResolved string) error {
	adpExe := filepath.Join(installRootResolved, "bin", "agent", "agent-data-plane.exe")
	if _, err := os.Stat(adpExe); err != nil {
		log.Debugf("ADP processes.d: skip write (agent-data-plane.exe stat %s: %v)", adpExe, err)
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		log.Debugf("ADP processes.d: cannot write, DatadogProgramFilesDir is empty (installRoot=%s)", installRootResolved)
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for ADP")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		log.Debugf("ADP processes.d: mkdir %s: %v", processesDir, err)
		return fmt.Errorf("create processes.d: %w", err)
	}

	installRootRepl := filepath.ToSlash(filepath.Clean(installRootResolved))
	etcRootRepl := filepath.ToSlash(filepath.Clean(paths.DatadogDataDir))
	log.Debugf("ADP processes.d: template replace __ADP_INSTALL_ROOT__=%q __ADP_ETC_ROOT__=%q",
		installRootRepl, etcRootRepl)

	config := embedded.ADPWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__ADP_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__ADP_ETC_ROOT__", etcRootRepl)

	path := filepath.Join(processesDir, adpProcmgrConfigFileName)
	log.Debugf("ADP processes.d: writing %q", path)
	return os.WriteFile(path, []byte(config), 0o644)
}

// RemoveADPProcmgrConfig removes the ADP processes.d YAML from the install layout and from
// legacy package-relative processes.d.
func RemoveADPProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", adpProcmgrConfigFileName)
		log.Debugf("ADP processes.d: remove %q", p)
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				log.Debugf("ADP processes.d: remove %q: not present", p)
			} else {
				log.Debugf("ADP processes.d: remove %q: %v", p, err)
				return err
			}
		}
	} else {
		log.Debugf("ADP processes.d: remove skip primary (DatadogProgramFilesDir is empty)")
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", adpProcmgrConfigFileName)
	log.Debugf("ADP processes.d: remove legacy %q", legacy)
	if err := os.Remove(legacy); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("ADP processes.d: remove legacy %q: not present", legacy)
		} else {
			log.Debugf("ADP processes.d: remove legacy %q: %v", legacy, err)
			return err
		}
	}
	return nil
}
