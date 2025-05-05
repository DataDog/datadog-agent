// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package executable provides information on the executable that started the process
// and utils to find other executables on the system
package executable

import (
	"os"
	"os/exec"
	"path/filepath"
)

func path(allowSymlinkFailure bool) (string, error) {
	here, err := os.Executable()
	if err != nil {
		return "", err
	}
	retstring, err := filepath.EvalSymlinks(here)
	if err != nil {
		if allowSymlinkFailure {
			// return no error here, since we're allowing the symlink to fail
			return here, nil
		}
	}
	return retstring, err

}

// Folder returns the folder under which the executable is located,
// after having resolved all symlinks to the executable.
// Unlike os.Executable and osext.ExecutableFolder, Folder will
// resolve the symlinks across all platforms.
func Folder() (string, error) {
	p, err := path(false)
	if err != nil {
		return "", err
	}

	return filepath.Dir(p), nil
}

// FolderAllowSymlinkFailure returns the folder under which the executable
// is located, without resolving symbolic links.
func FolderAllowSymlinkFailure() (string, error) {
	p, err := path(true)
	if err != nil {
		return "", err
	}

	return filepath.Dir(p), nil
}

// ResolvePath resolves the absolute path to the executable program
// with the given name in the argument. Returns error if the program's
// path cannot be resolved.
func ResolvePath(execName string) (string, error) {
	execPath, err := exec.LookPath(execName)
	if err != nil {
		return "", err
	}

	execAbsPath, err := filepath.Abs(execPath)
	if err != nil {
		return "", err
	}

	return execAbsPath, nil
}
