// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows,!darwin
// +build python

package app

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	pythonBin = "python"
)

func getRelPyPath() string {
	return filepath.Join("embedded", "bin", fmt.Sprintf("%s%s", pythonBin, pythonMajorVersion))
}

func getRelChecksPath() (string, error) {
	err := detectPythonMinorVersion()
	if err != nil {
		return "", err
	}

	pythonDir := fmt.Sprintf("%s%s.%s", pythonBin, pythonMajorVersion, pythonMinorVersion)
	return filepath.Join("embedded", "lib", pythonDir, "site-packages", "datadog_checks"), nil
}

func validateUser(allowRoot bool) error {
	if os.Geteuid() == 0 && !allowRoot {
		return fmt.Errorf("operation is disabled for root user. Please run this tool with the agent-running user or add '--allow-root/-r' to force")
	}
	return nil
}
