// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && python

package integrations

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	pythonBin                = "python"
	pythonMinorVersionScript = "import sys;print(sys.version_info[1])"
)

var (
	pythonMinorVersion string
)

func getRelPyPath() string {
	return filepath.Join("embedded", "bin", pythonBin+"3")
}

func getRelChecksPath(cliParams *cliParams) (string, error) {
	err := detectPythonMinorVersion(cliParams)
	if err != nil {
		return "", err
	}

	pythonDir := fmt.Sprintf("%s3.%s", pythonBin, pythonMinorVersion)
	return filepath.Join("embedded", "lib", pythonDir, "site-packages", "datadog_checks"), nil
}

func detectPythonMinorVersion(cliParams *cliParams) error {
	if pythonMinorVersion == "" {
		pythonPath, err := getCommandPython(cliParams.useSysPython)
		if err != nil {
			return err
		}

		versionCmd := exec.Command(pythonPath, "-c", pythonMinorVersionScript)
		minorVersion, err := versionCmd.Output()
		if err != nil {
			return err
		}

		pythonMinorVersion = strings.TrimSpace(string(minorVersion))
	}

	return nil
}
