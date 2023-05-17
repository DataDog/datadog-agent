// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && windows

package python

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// Any platform-specific initialization belongs here.
func initializePlatform() error {
	// On Windows, it's not uncommon to have a system-wide PYTHONPATH env var set.
	// Unset it, so our embedded python doesn't try to load things from the system.
	if !config.Datadog.GetBool("windows_use_pythonpath") {
		os.Unsetenv("PYTHONPATH")
	}

	return nil
}
