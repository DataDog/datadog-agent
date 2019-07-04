// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows
// +build python

package app

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	pythonBin = "python.exe"
)

var (
	relPyPath              = filepath.Join("..", "embedded2", pythonBin)
	relChecksPath          = filepath.Join("..", "embedded2", "Lib", "site-packages", "datadog_checks")
	relReqAgentReleasePath = filepath.Join("..", reqAgentReleaseFile)
	relConstraintsPath     = filepath.Join("..", constraintsFile)
)

func validateUser(allowRoot bool) error {
	elevated, _ := winutil.IsProcessElevated()
	if !elevated {
		return fmt.Errorf("Operation is not possible for unelevated process.")
	}
	return nil
}
