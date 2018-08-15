// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows
// +build cpython

package app

import (
	"os"
	"path/filepath"
)

const (
	pythonBin = "python.exe"
)

var (
	relPyPath            = pythonBin
	relConstraintsPath   = filepath.Join("..", constraintsFile)
	relTufConfigFilePath = filepath.Join("..", tufConfigFile)
	tufPipCachePath      = filepath.Join(os.Getenv("ProgramData"), "Datadog", "repositories", "cache")
)

func authorizedUser() bool {
	// TODO: implement something useful
	return true
}

func getTUFPipCachePath() (string, error) {
	if _, err := os.Stat(tufPipCachePath); err != nil {
		if os.IsNotExist(err) {
			return tufPipCachePath, err
		}
	}

	return tufPipCachePath, nil
}
