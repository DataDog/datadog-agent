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
)

const (
	pip = "pip"
)

var (
	relPipPath           = filepath.Join("..", "..", "embedded", "bin", pip)
	relConstraintsPath   = filepath.Join("..", "..", constraintsFile)
	relTufConfigFilePath = filepath.Join("..", "..", tufConfigFile)
	relTufPipCache       = filepath.Join("..", "..", "repositories", "cache")
)

func authorizedUser() bool {
	return (os.Geteuid() != 0)
}
