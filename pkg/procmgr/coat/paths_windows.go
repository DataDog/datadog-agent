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

func procmgrConfigPath(installRoot, configFile string) string {
	return filepath.Join(installRoot, processesDirRel, configFile)
}

// installMarkerPaths returns paths to check for an installed DDOT payload on Windows.
// Relative markers get .exe under the install root; the fleet packages path is appended
// when WindowsPackageName is set (see postInstallDDOTExtension / fleet layouts).
func installMarkerPaths(installRoot string, service MigratableService) []string {
	out := make([]string, 0, len(service.InstallMarkerRels)+1)
	for _, rel := range service.InstallMarkerRels {
		if rel == "" {
			continue
		}
		out = append(out, filepath.Join(installRoot, filepath.FromSlash(rel)+".exe"))
	}
	if service.WindowsPackageName == "" {
		return out
	}
	out = append(out, filepath.Join(
		windowsPackagesPath(),
		service.WindowsPackageName,
		"stable",
		"embedded",
		"bin",
		"otel-agent.exe",
	))
	return out
}

// windowsDatadogDataDir returns the agent data directory (MSI ConfigRoot when set,
// otherwise default ProgramData\Datadog), matching fleet installer layout for
// Installer\packages.
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
