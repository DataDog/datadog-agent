// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows
// +build cpython

package app

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const (
	pythonBin = "python"
)

var (
	relPyPath            = filepath.Join("..", "..", "embedded", "bin", pythonBin)
	relConstraintsPath   = filepath.Join("..", "..", constraintsFile)
	relTufConfigFilePath = filepath.Join("..", "..", tufConfigFile)
	relTufPipCache       = filepath.Join("..", "..", "repositories", "cache")
)

func authorizedUser() bool {
	return (os.Geteuid() != 0)
}

func getTUFPipCachePath() (string, error) {
	here, _ := executable.Folder()
	cPath := filepath.Join(here, relTufPipCache)

	if _, err := os.Stat(cPath); err != nil {
		if os.IsNotExist(err) {
			return cPath, err
		}
	}

	return cPath, nil
}
