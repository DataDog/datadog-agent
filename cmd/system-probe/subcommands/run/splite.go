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
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	apisecurity "github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/discovery/module/splite"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// skipSpliteHandoffEnv, when set in the environment, tells system-probe not to
// hand off to system-probe-lite for this run. system-probe-lite sets it when it
// re-execs back into system-probe so the process does not bounce straight back
// before remote config has had a chance to enable the extra module. The Rust
// binary hardcodes the same name; keep them in sync.
const skipSpliteHandoffEnv = "DD_SYSTEM_PROBE_SKIP_SPLITE_HANDOFF"

// shouldExecSPLite returns true if system-probe should exec into system-probe-lite.
// This is the case when use_system_probe_lite is enabled and only the discovery module is active.
func shouldExecSPLite(sysprobeConfig sysprobeconfig.Component, cfg *sysconfigtypes.Config) bool {
	// If system-probe-lite re-exec'd into us, do not hand back to it this run.
	if os.Getenv(skipSpliteHandoffEnv) != "" {
		return false
	}

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

	// Build args via splite package (source of truth for CLI format). ReexecArgs
	// carries our own invocation so system-probe-lite can transition back into a
	// full system-probe when a non-discovery module is enabled remotely.
	config := &splite.Config{
		Socket:     sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
		LogLevel:   sysprobeConfig.GetString("log_level"),
		LogFile:    sysprobeConfig.GetString("log_file"),
		PIDFile:    pidFilePath,
		ReexecArgs: append([]string{execPath}, os.Args[1:]...),
	}

	// When remote enablement is opted into, pass the agent IPC connection
	// parameters so system-probe-lite can poll remote config itself and
	// transition into a full system-probe when Live Debugger is enabled.
	if sysprobeConfig.GetBool("dynamic_instrumentation.remote_enable") {
		if ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog()); err != nil {
			log.Warnf("cannot resolve IPC address for remote enablement, remote config polling disabled: %s", err)
		} else {
			config.IPCAddress = ipcAddress
			config.IPCPort = pkgconfigsetup.GetIPCPort()
			config.AuthTokenPath = apisecurity.GetAuthTokenFilepath(pkgconfigsetup.Datadog())
			config.IPCCertPath = pkgconfigsetup.Datadog().GetString("ipc_cert_file_path")
		}
	}

	args := config.Args()

	return &spLiteExecCmd{
		Path: systemProbeLitePath,
		Args: append([]string{systemProbeLitePath}, args...),
		Env:  os.Environ(),
	}
}
