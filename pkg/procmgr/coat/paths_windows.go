// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package coat

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func agentInstallRoot() string {
	return defaultpaths.GetInstallPath()
}

func procmgrConfigPath(_ string, configFile string) string {
	return filepath.Join(windowsProcmgrConfigDir(), configFile)
}

func installMarkerPath(_ string, service MigratableService) string {
	if service.WindowsPackageName == "" {
		return ""
	}
	return filepath.Join(
		windowsPackagesPath(),
		service.WindowsPackageName,
		"stable",
		"embedded",
		"bin",
		"otel-agent.exe",
	)
}

// windowsDatadogDataDir returns the agent data directory (MSI ConfigRoot when set,
// otherwise default ProgramData\Datadog), matching fleet installer layout for
// Installer\packages and dd-procmgr\processes.d.
func windowsDatadogDataDir() string {
	if pd, err := winutil.GetProgramDataDir(); err == nil && pd != "" {
		return pd
	}
	if base := os.Getenv("ProgramData"); base != "" {
		return filepath.Join(base, "Datadog")
	}
	return filepath.Join(`C:\ProgramData`, "Datadog")
}

func windowsPackagesPath() string {
	return filepath.Join(windowsDatadogDataDir(), "Installer", "packages")
}

func windowsProcmgrConfigDir() string {
	return filepath.Join(windowsDatadogDataDir(), "dd-procmgr", processesDirRel)
}
