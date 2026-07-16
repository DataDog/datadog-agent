// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the systemd units for the installer.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

// TestGenerationIsUpToDate tests that the generated templates are up to date.
//
// You can update the templates by running `go generate` in the templates directory.
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}

	generated := filepath.Join(os.TempDir(), "gen")
	os.MkdirAll(generated, 0755)

	err := generate(generated)
	assert.NoError(t, err)
	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS := os.DirFS(checkedInGenDir())

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

// checkedInGenDir locates the checked-in gen/ directory (embedded/gen, a sibling
// of tmpl/, outside the gazelle-excluded tmpl/ tree). Its path relative to the
// working directory differs by test runner: `go test`/dda inv chdir into the
// source file's own directory (tmpl/), while Bazel's go_test chdirs into the
// BUILD file's directory (embedded/) since this target's srcs cross package
// directories.
func checkedInGenDir() string {
	for _, candidate := range []string{"gen", filepath.Join("..", "gen")} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "gen"
}
