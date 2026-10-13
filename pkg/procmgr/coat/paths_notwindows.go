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

// daemonLogLocation names where dd-procmgrd's own log can be read.
//
// Unlike the Windows service, the Unix daemon is started with no log file: it writes to stdout, and
// its systemd unit sets no StandardOutput, so the lines land in the journal. Nothing writes
// logs/dd-procmgr.log, so the report must not point a reader at it.
//
// The journal is named as a command to run rather than collected into the flare on purpose. The
// provider runs inside the core agent, which systemd starts as User=dd-agent, and packaging does
// not put that user in systemd-journal or adm. Collecting it here would usually yield an empty file
// or a permission error, which misleads a reader as surely as a path that does not exist.
func daemonLogLocation() string {
	return "the journal, via: journalctl -u datadog-agent-procmgr (or -exp for the experiment unit)"
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
