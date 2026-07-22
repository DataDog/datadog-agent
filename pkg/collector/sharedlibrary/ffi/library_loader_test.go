// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_NonExistentFile(t *testing.T) {
	loader, err := NewSharedLibraryLoader(t.TempDir())
	require.NoError(t, err)

	_, err = loader.Open("/path/that/does/not/exist.so")
	assert.Error(t, err)
}

func TestRun_NullLibraryPointer(t *testing.T) {
	loader, err := NewSharedLibraryLoader(t.TempDir())
	require.NoError(t, err)

	err = loader.Run(nil, "", "", "")
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")

	_, err = loader.Version(nil)
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")

	err = loader.Close(nil)
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")
}

func TestRun_LibraryWithNullSymbols(t *testing.T) {
	loader, err := NewSharedLibraryLoader(t.TempDir())
	require.NoError(t, err)

	lib := NewLibraryWithNullSymbols()

	err = loader.Run(lib, "", "", "")
	assert.EqualError(t, err, "Failed to run check: pointer to 'Run' symbol of the shared library is NULL")

	_, err = loader.Version(lib)
	assert.EqualError(t, err, "Failed to get version: pointer to 'Version' symbol of the shared library is NULL")
}

func TestComputeLibraryPath_Valid(t *testing.T) {
	folder := t.TempDir()
	loader, err := NewSharedLibraryLoader(folder)
	require.NoError(t, err)

	cases := []string{
		"mycheck",
		"my_check",
		"my-check",
		"no-run-symbol",
		"check123",
		"MY_CHECK",
		"a",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			libPath, err := loader.ComputeLibraryPath(name)
			require.NoError(t, err)
			expected := path.Join(folder, "libdatadog-agent-"+name+"."+getLibExtension())
			assert.Equal(t, expected, libPath)
			assert.True(t, strings.HasPrefix(libPath, folder+"/"), "libPath %q should be inside %q", libPath, folder)
		})
	}
}

func TestComputeLibraryPath_RejectsInvalidNames(t *testing.T) {
	loader, err := NewSharedLibraryLoader(t.TempDir())
	require.NoError(t, err)

	cases := []string{
		// empty
		"",
		// path traversal with slashes
		"foo/../../tmp/baz",
		"../baz",
		"foo/bar",
		`foo\bar`,
		`..\baz`,
		"/etc/passwd",
		// NUL byte
		"foo\x00bar",
		// dots (allowed by old blocklist, rejected by allowlist)
		"my.check",
		".",
		"..",
		// starts with non-alphanumeric
		"-mycheck",
		"_mycheck",
		// spaces and special characters
		"my check",
		"my@check",
		"my+check",
		// Windows drive prefixes
		`C:\evil`,
		"C:evil",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			libPath, err := loader.ComputeLibraryPath(name)
			require.Error(t, err)
			assert.Empty(t, libPath)
		})
	}
}

func TestIsPathConfined(t *testing.T) {
	folder := "/etc/datadog-agent/checks.d"

	assert.True(t, isPathConfined("/etc/datadog-agent/checks.d/libdatadog-agent-foo.so", folder))

	assert.False(t, isPathConfined("/tmp/evil.so", folder))
	assert.False(t, isPathConfined("/etc/datadog-agent/checks.d/libdatadog-agent-foo/../../evil.so", folder))
	assert.False(t, isPathConfined("/etc/evil.so", folder))
}
