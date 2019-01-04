// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package executable provides information on the executable that started the process
package executable

import (
	"path/filepath"

	// TODO: Use the built-in "os" package as soon as it implements `Executable()`
	// consistently across all platforms
	"github.com/kardianos/osext"
)

func path() (string, error) {
	here, err := osext.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(here)
}

// Folder returns the folder under which the executable is located,
// after having resolved all symlinks to the executable.
// Unlike os.Executable and osext.ExecutableFolder, Folder will
// resolve the symlinks across all platforms.
func Folder() (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}

	return filepath.Dir(p), nil
}
