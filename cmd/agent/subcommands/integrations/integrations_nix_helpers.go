// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && python
// +build !windows,python

package integrations

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	pythonBin                = "python"
	pythonMinorVersionScript = "import sys;print(sys.version_info[1])"
)

func getRelPyPath(version string) string {
	return filepath.Join("embedded", "bin", fmt.Sprintf("%s%s", pythonBin, version))
}

func getRelChecksPath(cliParams *cliParams) (string, error) {
	pythonMinorVersion, err := detectPythonMinorVersion(cliParams)
	if err != nil {
		return "", err
	}

	pythonDir := fmt.Sprintf("%s%s.%s", pythonBin, cliParams.pythonMajorVersion, pythonMinorVersion)
	return filepath.Join("embedded", "lib", pythonDir, "site-packages", "datadog_checks"), nil
}

func detectPythonMinorVersion(cliParams *cliParams) (string, error) {
	minorVersion, err := cliParams.python().runCommand(pythonMinorVersionScript)

	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(minorVersion)), nil
}
