// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain puts the Bazel-provided interop DLL directory on PATH so
// LoadLibrary("libdatadog-interop.dll") finds it. Non-Bazel runs (dda inv)
// leave INTEROP_DLL_PATH unset and already put the repo root (with the DLL) on PATH.
func TestMain(m *testing.M) {
	if p := os.Getenv("INTEROP_DLL_PATH"); p != "" {
		p = filepath.FromSlash(p)
		if !filepath.IsAbs(p) {
			if root := os.Getenv("RUNFILES_DIR"); root != "" {
				p = filepath.Join(root, p)
			} else if root := os.Getenv("TEST_SRCDIR"); root != "" {
				p = filepath.Join(root, p)
			}
		}
		os.Setenv("PATH", filepath.Dir(p)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.Exit(m.Run())
}
