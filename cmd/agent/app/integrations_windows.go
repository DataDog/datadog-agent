// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows
// +build cpython python

package app

import (
	"path/filepath"
)

const (
	pythonBin = "python.exe"
)

var (
	relPyPath              = pythonBin
	relChecksPath          = filepath.Join("Lib", "site-packages", "datadog_checks")
	relReqAgentReleasePath = filepath.Join("..", reqAgentReleaseFile)
	relConstraintsPath     = filepath.Join("..", constraintsFile)
)

func authorizedUser() bool {
	// TODO: implement something useful
	return true
}
