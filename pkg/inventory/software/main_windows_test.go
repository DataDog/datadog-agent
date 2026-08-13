// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
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

		// Bazel may materialize the runfiles entry as a directory junction pointing at
		// the real DLL instead of as a file. Windows then reports the path as a
		// directory and LoadLibrary refuses the image with ERROR_ACCESS_DENIED, which
		// surfaces as an opaque "Access is denied" panic deep inside the collector.
		// Resolve the reparse point first: os.Stat fails on such an entry and
		// filepath.EvalSymlinks returns it unchanged, so os.Readlink is what works.
		if fi, err := os.Lstat(p); err == nil && !fi.Mode().IsRegular() {
			target, rerr := os.Readlink(p)
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "interop DLL %q is not a regular file and could not be resolved: %v\n", p, rerr)
				os.Exit(1)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(p), target)
			}
			p = target
		}
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			fmt.Fprintf(os.Stderr, "interop DLL %q is not a regular file: %v\n", p, err)
			os.Exit(1)
		}

		os.Setenv("PATH", filepath.Dir(p)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.Exit(m.Run())
}
