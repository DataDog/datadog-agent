// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package landlock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAllowedPathsMapsModes(t *testing.T) {
	readOnly := t.TempDir()
	readWrite := t.TempDir()
	unsuffixed := t.TempDir()

	rules, err := parseAllowedPaths([]string{readOnly + ":ro", readWrite + ":rw", unsuffixed})
	require.NoError(t, err)
	require.Equal(t, []pathRule{
		{path: readOnly, mode: accessReadOnly},
		{path: readWrite, mode: accessReadWrite},
		{path: unsuffixed, mode: accessReadOnly},
	}, rules)
}

func TestParseAllowedPathsRejectsRelativeAndEmptyPaths(t *testing.T) {
	for _, allowedPath := range []string{"", "relative", "relative:rw"} {
		t.Run(allowedPath, func(t *testing.T) {
			_, err := parseAllowedPaths([]string{allowedPath})
			require.Error(t, err)
		})
	}
}

func TestParseAllowedPathsPreservesNestedModesForDescriptorValidation(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	rules, err := parseAllowedPaths([]string{parent + ":rw", child + ":ro"})
	require.NoError(t, err)
	require.Equal(t, []pathRule{
		{path: parent, mode: accessReadWrite},
		{path: child, mode: accessReadOnly},
	}, rules)
}

func TestParseAllowedPathsPreservesExistingLiteralSuffixPath(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("colons are not valid in Windows path components")
	}
	literal := filepath.Join(t.TempDir(), "literal:rw")
	require.NoError(t, os.Mkdir(literal, 0o700))

	rules, err := parseAllowedPaths([]string{literal})
	require.NoError(t, err)
	require.Equal(t, pathRule{path: literal, mode: accessReadOnly}, rules[0])
}
