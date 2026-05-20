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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.NoError(t, os.MkdirAll(generated, 0755))

	require.NoError(t, generate(generated))
	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS := committedGenFS(t)

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

func committedGenFS(t *testing.T) fs.FS {
	t.Helper()
	dir, err := committedGenDir()
	require.NoError(t, err, "committed tmpl/gen tree not found (run from repo checkout)")
	return os.DirFS(dir)
}

const committedGenRel = "pkg/fleet/installer/packages/embedded/tmpl/gen"

func committedGenDir() (string, error) {
	candidates := []string{}

	if _, file, _, ok := runtime.Caller(1); ok {
		srcDir := filepath.Dir(file)
		srcDir = strings.TrimPrefix(srcDir, "github.com/DataDog/datadog-agent/")
		candidates = append(candidates, filepath.Join(srcDir, "..", "tmpl", "gen"))
		if root, err := repoRoot(); err == nil {
			candidates = append(candidates, filepath.Join(root, srcDir, "..", "tmpl", "gen"))
		}
	}

	if root, err := repoRoot(); err == nil {
		candidates = append(candidates, filepath.Join(root, committedGenRel))
	}

	if src := os.Getenv("TEST_SRCDIR"); src != "" {
		candidates = append(candidates,
			filepath.Join(src, "_main", committedGenRel),
			filepath.Join(src, committedGenRel),
		)
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() || d.Name() != "gen" {
				return nil
			}
			if filepath.Base(filepath.Dir(path)) == "tmpl" &&
				filepath.Base(filepath.Dir(filepath.Dir(path))) == "embedded" {
				candidates = append(candidates, path)
			}
			return nil
		})
	}

	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", os.ErrNotExist
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, committedGenRel)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
