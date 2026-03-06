// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

import (
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
