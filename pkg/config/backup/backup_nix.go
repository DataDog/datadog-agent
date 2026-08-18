// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package backup

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// fileOwnership returns the real on-disk uid/gid of a file. Recording the real
// ownership forces any future restore path to confront that it cannot
// faithfully restore a root-owned file from a dd-agent-written archive.
func fileOwnership(info fs.FileInfo) (uid, gid int) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return 0, 0
}

// isExperiment reports whether the resolved config directory is the Fleet
// installer experiment directory. On non-Windows the installer names the
// experiment directory with a -exp suffix (/etc/datadog-agent-exp), which is
// the only signal available to the core Agent. On Windows the live config
// directory is always the stable one, so this is always false there.
func isExperiment(srcDir string) bool {
	return strings.HasSuffix(srcDir, "-exp")
}

// readDeploymentID reads the Fleet installer state file if present.
func readDeploymentID(srcDir string) string {
	data, err := os.ReadFile(filepath.Join(srcDir, deploymentIDFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
