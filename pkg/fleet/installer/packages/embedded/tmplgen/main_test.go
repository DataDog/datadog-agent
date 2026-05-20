// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the systemd units for the installer.
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

// TestGenerationIsUpToDate tests that the generated templates are up to date.
//
// You can update the templates by running `go generate` in the tmplgen directory.
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}

	// Use a fresh directory: generate() only overwrites known filenames; a fixed
	// path under os.TempDir() would keep removed artifacts (e.g. renamed YAML)
	// and break comparison with the embedded gen tree.
	generated := filepath.Join(t.TempDir(), "gen")
	if err := os.MkdirAll(generated, 0755); err != nil {
		t.Fatal(err)
	}

	err := generate(generated)
	assert.NoError(t, err)
	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS := committedGenFS(t)

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

func committedGenFS(t *testing.T) fs.FS {
	t.Helper()
	for _, dir := range committedGenDirs() {
		if _, err := os.Stat(dir); err == nil {
			return os.DirFS(dir)
		}
	}
	t.Fatal("committed tmpl/gen tree not found (run from repo or Bazel test runfiles)")
	return nil
}

func committedGenDirs() []string {
	dirs := []string{}
	if _, file, _, ok := runtime.Caller(0); ok {
		dirs = append(dirs, filepath.Join(filepath.Dir(file), "..", "tmpl", "gen"))
	}
	if src := os.Getenv("TEST_SRCDIR"); src != "" {
		dirs = append(dirs,
			filepath.Join(src, "_main/pkg/fleet/installer/packages/embedded/tmpl/gen"),
			filepath.Join(src, "pkg/fleet/installer/packages/embedded/tmpl/gen"),
		)
		// Bazel runfiles layout varies; locate embedded/tmpl/gen under TEST_SRCDIR.
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() || d.Name() != "gen" {
				return nil
			}
			if filepath.Base(filepath.Dir(path)) == "tmpl" &&
				filepath.Base(filepath.Dir(filepath.Dir(path))) == "embedded" {
				dirs = append(dirs, path)
			}
			return nil
		})
	}
	return dirs
}
