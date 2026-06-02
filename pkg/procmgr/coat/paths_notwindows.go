// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package coat

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

func agentInstallRoot() string {
	return filepath.Clean(filepath.Join(defaultpaths.GetInstallPath(), "..", ".."))
}

func procmgrConfigPath(installRoot, configFile string) string {
	return filepath.Join(installRoot, processesDirRel, configFile)
}

func installMarkerPath(installRoot string, service MigratableService) string {
	return filepath.Join(installRoot, service.InstallMarkerRel)
}
