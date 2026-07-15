// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// installRootProcmgrSpec describes a processes.d config whose binary and YAML both
// live under the same MSI Program Files install root (PAR, ADP). DDOT is different:
// its binary is under the fleet package path while processes.d stays on the install root.
type installRootProcmgrSpec struct {
	logLabel          string
	binaryRelPath     string
	configFileName    string
	embeddedConfig    string
	placeholderPrefix string
}

func (s installRootProcmgrSpec) configRelPath() string {
	return filepath.ToSlash(filepath.Join("processes.d", s.configFileName))
}

func (s installRootProcmgrSpec) renderConfig(installRootResolved string) string {
	installRootRepl := filepath.ToSlash(filepath.Clean(installRootResolved))
	etcRootRepl := filepath.ToSlash(filepath.Clean(paths.DatadogDataDir))
	log.Debugf("%s processes.d: template replace __%s_INSTALL_ROOT__=%q __%s_ETC_ROOT__=%q",
		s.logLabel, s.placeholderPrefix, installRootRepl,
		s.placeholderPrefix, etcRootRepl)

	config := s.embeddedConfig
	config = strings.ReplaceAll(config, "__"+s.placeholderPrefix+"_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__"+s.placeholderPrefix+"_ETC_ROOT__", etcRootRepl)
	return config
}

func writeInstallRootProcmgrConfig(installRootResolved string, spec installRootProcmgrSpec) error {
	installRoot, err := os.OpenRoot(installRootResolved)
	if err != nil {
		return fmt.Errorf("open install root: %w", err)
	}
	defer installRoot.Close()

	binaryPath := filepath.Join(installRootResolved, filepath.FromSlash(spec.binaryRelPath))
	if _, err := installRoot.Stat(spec.binaryRelPath); err != nil {
		log.Debugf("%s processes.d: skip write (%s stat %s: %v)", spec.logLabel, filepath.Base(spec.binaryRelPath), binaryPath, err)
		return nil
	}

	processesDir := filepath.Join(installRootResolved, "processes.d")
	if err := installRoot.MkdirAll("processes.d", 0o755); err != nil {
		log.Debugf("%s processes.d: mkdir %s: %v", spec.logLabel, processesDir, err)
		return fmt.Errorf("create processes.d: %w", err)
	}

	configPath := filepath.Join(processesDir, spec.configFileName)
	log.Debugf("%s processes.d: writing %q", spec.logLabel, configPath)
	if err := installRoot.WriteFile(spec.configRelPath(), []byte(spec.renderConfig(installRootResolved)), 0o644); err != nil {
		return err
	}
	return nil
}

func removeInstallRootProcmgrConfig(installRootResolved string, spec installRootProcmgrSpec) error {
	installRoot, err := os.OpenRoot(installRootResolved)
	if err != nil {
		return fmt.Errorf("open install root: %w", err)
	}
	defer installRoot.Close()

	configPath := filepath.Join(installRootResolved, "processes.d", spec.configFileName)
	if err := removeInstallRootProcmgrConfigAtRoot(installRoot, configPath, spec); err != nil {
		return err
	}

	removeEmptyProcessesDir(installRootResolved)
	return nil
}

func removeInstallRootProcmgrConfigAtRoot(root *os.Root, absPathForLog string, spec installRootProcmgrSpec) error {
	log.Debugf("%s processes.d: remove %q", spec.logLabel, absPathForLog)
	if err := root.Remove(spec.configRelPath()); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("%s processes.d: remove %q: not present", spec.logLabel, absPathForLog)
			return nil
		}
		log.Debugf("%s processes.d: remove %q: %v", spec.logLabel, absPathForLog, err)
		return err
	}
	return nil
}
