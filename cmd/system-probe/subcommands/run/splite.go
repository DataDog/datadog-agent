// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package run

import (
	"os"
	"path/filepath"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/discovery/module/splite"
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

	// Exec system-probe-lite if only the discovery module is enabled
	return cfg.Enabled && len(cfg.EnabledModules) == 1 && cfg.ModuleIsEnabled(systemprobeconfig.DiscoveryModule)
}

// maybeSPLite checks if system-probe should exec into system-probe-lite,
// and if so, returns the resolved command. Returns nil if splite is not
// applicable or the binary was not found.
func maybeSPLite(sysprobeConfig sysprobeconfig.Component, pidFilePath string, log log.Component) *spLiteExecCmd {
	cfg := sysprobeConfig.SysProbeObject()
	if !shouldExecSPLite(sysprobeConfig, cfg) {
		return nil
	}

	// Resolve binary path — system-probe-lite is expected next to system-probe
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

	// Build args via splite package (source of truth for CLI format)
	args := (&splite.Config{
		Socket:   sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
		LogLevel: sysprobeConfig.GetString("log_level"),
		LogFile:  sysprobeConfig.GetString("log_file"),
		PIDFile:  pidFilePath,
	}).Args()

	return &spLiteExecCmd{
		Path: systemProbeLitePath,
		Args: append([]string{systemProbeLitePath}, args...),
		Env:  os.Environ(),
	}
}
