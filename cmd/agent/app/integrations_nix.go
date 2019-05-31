// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows,!darwin
// +build python

package app

import (
	"os"
	"path/filepath"
)

const (
	pythonBin = "python2"
)

var (
	relPyPath              = filepath.Join("..", "..", "embedded", "bin", pythonBin)
	relChecksPath          = filepath.Join("..", "..", "embedded", "lib", "python2.7", "site-packages", "datadog_checks")
	relReqAgentReleasePath = filepath.Join("..", "..", reqAgentReleaseFile)
	relConstraintsPath     = filepath.Join("..", "..", constraintsFile)
)

func authorizedUser() bool {
	return (os.Geteuid() != 0)
}

func isIntegrationUser() bool {
	return true
}
