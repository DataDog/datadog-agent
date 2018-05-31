// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows
// +build cpython

package app

import (
	"path/filepath"
)

const (
	pip = "pip.exe"
)

var (
	relPipPath           = filepath.Join("Scripts", pip)
	relConstraintsPath   = filepath.Join("..", constraintsFile)
	relTufConfigFilePath = filepath.Join("..", tufConfigFile)
	relTufPipCache       = filepath.Join("..", "repositories", "cache")
)

func authorizedUser() bool {
	// TODO: implement something useful
	return true
}
