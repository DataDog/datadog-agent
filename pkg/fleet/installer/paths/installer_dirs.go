// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package paths

import (
	"fmt"
	"os"
)

// EnsureInstallerDirectories creates the installer data, packages, configs, tmp,
// and run directories if they do not exist.
func EnsureInstallerDirectories() error {
	err := EnsureInstallerDataDir()
	if err != nil {
		return fmt.Errorf("could not ensure installer data directory permissions: %w", err)
	}

	err = os.MkdirAll(PackagesPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating packages directory: %w", err)
	}
	err = os.MkdirAll(ConfigsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating configs directory: %w", err)
	}
	err = os.MkdirAll(RootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating tmp directory: %w", err)
	}
	err = os.MkdirAll(RunPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating run directory: %w", err)
	}

	return nil
}
