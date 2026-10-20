// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAllowedPathModeNonWindowsStripsMissingSuffixPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "policy")

	path, mode := resolveAllowedPathMode(base + ":rw")
	assert.Equal(t, base, path)
	assert.Equal(t, pathModeReadWrite, mode)

	path, mode = resolveAllowedPathMode(base + ":ro")
	assert.Equal(t, base, path)
	assert.Equal(t, pathModeReadOnly, mode)
}

func TestResolveAllowedPathModeNonWindowsPreservesExistingLiteralSuffixPath(t *testing.T) {
	dir := t.TempDir()

	for _, suffix := range []string{":rw", ":ro"} {
		t.Run(suffix, func(t *testing.T) {
			literal := filepath.Join(dir, "policy"+suffix)
			require.NoError(t, os.Mkdir(literal, 0755))

			path, mode := resolveAllowedPathMode(literal)
			assert.Equal(t, literal, path)
			assert.Equal(t, pathModeReadOnly, mode)
		})
	}
}

func TestResolveAllowedPathModeNonWindowsPreservesPathWhenLstatCannotProveAbsence(t *testing.T) {
	pathWithInvalidName := "policy\x00:rw"

	path, mode := resolveAllowedPathMode(pathWithInvalidName)
	assert.Equal(t, pathWithInvalidName, path)
	assert.Equal(t, pathModeReadOnly, mode)
}
