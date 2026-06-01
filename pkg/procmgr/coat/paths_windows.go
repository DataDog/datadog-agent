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

func windowsProgramData() string {
	if base := os.Getenv("ProgramData"); base != "" {
		return base
	}
	return `C:\ProgramData`
}

func windowsPackagesPath() string {
	return filepath.Join(windowsProgramData(), "Datadog", "Installer", "packages")
}

func windowsProcmgrConfigDir() string {
	return filepath.Join(windowsProgramData(), "Datadog", "dd-procmgr", processesDirRel)
}
