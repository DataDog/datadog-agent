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
	return defaultpaths.GetInstallPath()
}

func procmgrConfigPath(installRoot, configFile string) string {
	return filepath.Join(installRoot, processesDirRel, configFile)
}

// installMarkerPaths returns paths to check for an installed payload on !windows.
func installMarkerPaths(installRoot string, service MigratableService) []string {
	out := make([]string, 0, len(service.InstallMarkerRels))
	for _, rel := range service.InstallMarkerRels {
		if rel == "" {
			continue
		}
		out = append(out, filepath.Join(installRoot, filepath.FromSlash(rel)))
	}
	return out
}
