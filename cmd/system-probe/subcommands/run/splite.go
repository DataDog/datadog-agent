// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package run

import (
	"os"
	"path/filepath"
	"syscall"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// shouldExecSPLite returns true if system-probe should exec into system-probe-lite.
// This is the case when use_system_probe_lite is enabled and the full system-probe is not needed
// (either no modules are enabled, or only the discovery module is enabled).
func shouldExecSPLite(sysprobeConfig sysprobeconfig.Component, cfg *sysconfigtypes.Config) bool {
	if !sysprobeConfig.GetBool("discovery.use_system_probe_lite") {
		return false
	}

	// Don't exec system-probe-lite if an external system-probe is managing things
	if cfg.ExternalSystemProbe {
		return false
	}

	// If discovery is explicitly disabled and nothing else is enabled, just exit cleanly
	if sysprobeConfig.IsConfigured("discovery.enabled") && !sysprobeConfig.GetBool("discovery.enabled") && !cfg.Enabled {
		return false
	}

	// Exec system-probe-lite if no modules are enabled, or only discovery is enabled
	return !cfg.Enabled || (len(cfg.EnabledModules) == 1 && cfg.ModuleIsEnabled(systemprobeconfig.DiscoveryModule))
}

// spLiteExecCmd holds the resolved path and arguments for execing into system-probe-lite.
type spLiteExecCmd struct {
	Path string
	Args []string
	Env  []string
}

// buildSPLiteArgs builds the command-line arguments for the system-probe-lite binary.
func buildSPLiteArgs(sysprobeConfig sysprobeconfig.Component, pidFilePath string) []string {
	args := []string{"system-probe-lite",
		"--socket", sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
		"--log-level", sysprobeConfig.GetString("log_level"),
		"--log-file", sysprobeConfig.GetString("log_file"),
	}
	if pidFilePath != "" {
		args = append(args, "--pid", pidFilePath)
	}
	return args
}

// resolveSPLiteExecCmd resolves the system-probe-lite binary path and builds the exec arguments.
// Returns nil if the binary cannot be found (graceful fallback).
func resolveSPLiteExecCmd(sysprobeConfig sysprobeconfig.Component, pidFilePath string, log log.Component) *spLiteExecCmd {
	// system-probe-lite binary is expected in the same directory as system-probe
	execPath, err := os.Executable()
	if err != nil {
		log.Warnf("cannot determine system-probe executable path: %s, falling back to running discovery in system-probe", err)
		return nil
	}
	systemProbeLitePath := filepath.Join(filepath.Dir(execPath), "system-probe-lite")

	if _, err := os.Stat(systemProbeLitePath); err != nil {
		log.Warnf("system-probe-lite binary not found at %s: %s, falling back to running discovery in system-probe", systemProbeLitePath, err)
		return nil
	}

	return &spLiteExecCmd{
		Path: systemProbeLitePath,
		Args: buildSPLiteArgs(sysprobeConfig, pidFilePath),
		Env:  os.Environ(),
	}
}

// maybeSPLite checks if system-probe should exec into system-probe-lite,
// and if so, replaces the current process. If splite is not applicable,
// the binary was not found, or exec fails, it returns and system-probe
// continues with normal startup.
func maybeSPLite(sysprobeConfig sysprobeconfig.Component, pidFilePath string, log log.Component) {
	cfg := sysprobeConfig.SysProbeObject()
	if !shouldExecSPLite(sysprobeConfig, cfg) {
		return
	}
	log.Info("only discovery module enabled with use_system_probe_lite=true, will exec into system-probe-lite")
	cmd := resolveSPLiteExecCmd(sysprobeConfig, pidFilePath, log)
	if cmd == nil {
		return
	}
	log.Infof("execing into system-probe-lite: %s %v", cmd.Path, cmd.Args)
	log.Flush()
	if err := syscall.Exec(cmd.Path, cmd.Args, cmd.Env); err != nil {
		log.Warnf("failed to exec into system-probe-lite: %s, falling back to running discovery in system-probe", err)
	}
}
