// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package configbackupimpl

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fileOwnership returns the real on-disk uid/gid of a file. Windows has no
// meaningful uid/gid, so this returns zeroes.
func fileOwnership(info fs.FileInfo) (uid, gid int) {
	return 0, 0
}

// isExperiment reports whether the resolved config directory is the Fleet
// installer experiment directory. On Windows the live config directory is
// always the stable one (C:\\ProgramData\\Datadog), for stable and experiment
// alike, so this is always false. Experiment-ness on Windows must come from
// installer state (the .deployment-id value), not from the directory path.
func isExperiment(srcDir string) bool {
	return false
}

// readDeploymentID reads the Fleet installer state file if present.
func readDeploymentID(srcDir string) string {
	data, err := os.ReadFile(filepath.Join(srcDir, deploymentIDFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
