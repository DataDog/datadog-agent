// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && windows

package python

import (
	"os"
	"path/filepath"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// Any platform-specific initialization belongs here.
func initializePlatform() error {
	// On Windows, it's not uncommon to have a system-wide PYTHONPATH env var set.
	// Unset it, so our embedded python doesn't try to load things from the system.
	if !pkgconfigsetup.Datadog().GetBool("windows_use_pythonpath") {
		os.Unsetenv("PYTHONPATH")
	}

	// only use cache file when not admin
	admin, _ := winutil.IsUserAnAdmin()
	if admin {
		return nil
	}

	// get program data directory and set PYTHONPYCACHEPREFIX
	pd, err := winutil.GetProgramDataDir()
	if err != nil {
		return err
	}
	pycache := filepath.Join(pd, "python-cache")

	// check if path exists
	if _, err := os.Stat(pycache); os.IsNotExist(err) {
		// create the directory
		if err := os.MkdirAll(pycache, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	os.Setenv("PYTHONPYCACHEPREFIX", pycache)

	return nil
}
