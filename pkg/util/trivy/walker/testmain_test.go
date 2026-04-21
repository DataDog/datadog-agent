// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package walker

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestMain copies the testdata tree into a real temp directory before running
// tests. Bazel provides runfiles as symlinks that escape the os.OpenRoot
// boundary ("path escapes from parent"). Real copies ensure os.Root accepts
// the files without restriction.
func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

// runMain does the actual setup and teardown so that defers execute before
// os.Exit is called (os.Exit does not run deferred functions).
func runMain(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "walker_test_")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	if err := copyTestdata("testdata", filepath.Join(tmp, "testdata")); err != nil {
		panic(err)
	}

	if err := os.Chdir(tmp); err != nil {
		panic(err)
	}

	return m.Run()
}

func copyTestdata(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		// os.Open follows symlinks, so Bazel runfile symlinks are read correctly.
		return copyFileContents(path, target)
	})
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
