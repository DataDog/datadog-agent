// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package paths

import (
	"path/filepath"
	"testing"
)

// SetupTestPaths points the paths at a temporary directory for the duration of the test, and returns
// the configuration directory. Tests use it so that they do not use the real C:\ProgramData\Datadog,
// which has to be owned by Administrators or SYSTEM.
func SetupTestPaths(t *testing.T) string {
	// Registered before t.Setenv so that it runs after the environment is restored, cleanup
	// functions run in reverse order.
	t.Cleanup(initPaths)

	dir := filepath.Join(t.TempDir(), "Datadog")
	t.Setenv("DD_APPLICATIONDATADIRECTORY", dir)
	initPaths()

	return dir
}
