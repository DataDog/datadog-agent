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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ddotProcmgrConfigFileName = "datadog-agent-ddot.yaml"

// WriteDDOTProcmgrConfig writes datadog-agent-ddot.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func WriteDDOTProcmgrConfig(installRootResolved string) error {
	otelExe := filepath.Join(installRootResolved, "ext", "ddot", "embedded", "bin", "otel-agent.exe")
	if _, err := os.Stat(otelExe); err != nil {
		log.Debugf("DDOT processes.d: skip write (otel-agent.exe stat %s: %v)", otelExe, err)
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		log.Debugf("DDOT processes.d: cannot write, DatadogProgramFilesDir is empty (installRoot=%s)", installRootResolved)
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for DDOT")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		log.Debugf("DDOT processes.d: mkdir %s: %v", processesDir, err)
		return fmt.Errorf("create processes.d: %w", err)
	}

	installRootRepl := filepath.ToSlash(filepath.Clean(installRootResolved))
	etcRootRepl := filepath.ToSlash(filepath.Clean(paths.DatadogDataDir))
	log.Debugf("DDOT processes.d: template replace __DDOT_INSTALL_ROOT__=%q __DDOT_ETC_ROOT__=%q",
		installRootRepl, etcRootRepl)

	config := embedded.DDOTWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__DDOT_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__DDOT_ETC_ROOT__", etcRootRepl)

	path := filepath.Join(processesDir, ddotProcmgrConfigFileName)
	log.Debugf("DDOT processes.d: writing %q", path)
	return os.WriteFile(path, []byte(config), 0o644)
}

// RemoveDDOTProcmgrConfig removes the DDOT processes.d YAML from the install layout and from
// legacy package-relative processes.d.
func RemoveDDOTProcmgrConfig(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", ddotProcmgrConfigFileName)
		log.Debugf("DDOT processes.d: remove %q", p)
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				log.Debugf("DDOT processes.d: remove %q: not present", p)
			} else {
				log.Debugf("DDOT processes.d: remove %q: %v", p, err)
				return err
			}
		}
	} else {
		log.Debugf("DDOT processes.d: remove skip primary (DatadogProgramFilesDir is empty)")
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", ddotProcmgrConfigFileName)
	log.Debugf("DDOT processes.d: remove legacy %q", legacy)
	if err := os.Remove(legacy); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("DDOT processes.d: remove legacy %q: not present", legacy)
		} else {
			log.Debugf("DDOT processes.d: remove legacy %q: %v", legacy, err)
			return err
		}
	}
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		removeEmptyProcessesDir(installPF)
	}
	return nil
}
