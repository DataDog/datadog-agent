// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GetExecutable returns the path to the current executable.
// Most of the time it'll be a wrapper around os.Executable, but on the rare case it fails (e.g. /proc not dumpable, hidepid...)
// we'll fall back on the absolute path of the current process.
func GetExecutable() (string, error) {
	executable, err1 := os.Executable()
	if err1 == nil {
		return executable, nil
	}

	// Get the absolute path of the current process from argv[0]
	executable, err2 := fromArgv0()
	if err2 == nil {
		return executable, nil
	}
	return "", fmt.Errorf("failed to get executable: %w, %w", err1, err2)
}

func fromArgv0() (string, error) {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return "", errors.New("empty argv[0]")
	}

	// LookPath handles both absolute and relative paths
	path, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}

	// Resolve symlinks to match os.Executable() behavior
	return filepath.EvalSymlinks(path)
}
