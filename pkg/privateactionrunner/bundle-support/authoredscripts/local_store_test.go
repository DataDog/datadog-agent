// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testArtifactSHA256 = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"

func testDescriptor() Descriptor {
	return Descriptor{
		Package: "com.datadoghq.authoredscripts.test",
		Version: "0.0.1",
		URL:     "oci://registry.example.test/authored-script@sha256:" + testArtifactSHA256,
		SHA256:  testArtifactSHA256,
	}
}

func TestNewLocalStore(t *testing.T) {
	t.Run("requires a directory", func(t *testing.T) {
		store, err := NewLocalStore("")

		require.ErrorContains(t, err, "directory is required")
		assert.Nil(t, store)
	})

	t.Run("requires an absolute directory", func(t *testing.T) {
		store, err := NewLocalStore("relative/path")

		require.ErrorContains(t, err, "is not absolute")
		assert.Nil(t, store)
	})

	t.Run("cleans the directory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache", "..", "store")

		store, err := NewLocalStore(root)

		require.NoError(t, err)
		assert.Equal(t, filepath.Clean(root), store.rootDirectory)
	})
}

func TestLocalStoreOpen(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStore(root)
	require.NoError(t, err)

	artifactDirectory := filepath.Join(root, testArtifactSHA256)
	require.NoError(t, os.MkdirAll(filepath.Join(artifactDirectory, scriptDirectory), 0o700))

	artifact, err := store.Open(testDescriptor())

	require.NoError(t, err)
	assert.Equal(t, artifactDirectory, artifact.Directory)
}

func TestLocalStoreOpenMissingArtifact(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)

	artifact, err := store.Open(testDescriptor())

	require.ErrorIs(t, err, ErrLocalArtifactNotFound)
	assert.Empty(t, artifact)
}

func TestLocalStoreOpenRejectsScriptFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStore(root)
	require.NoError(t, err)

	artifactDirectory := filepath.Join(root, testArtifactSHA256)
	require.NoError(t, os.MkdirAll(artifactDirectory, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(artifactDirectory, scriptDirectory), nil, 0o600))

	_, err = store.Open(testDescriptor())

	require.ErrorContains(t, err, "is not a directory")
}

func TestLocalStoreOpenValidatesDescriptor(t *testing.T) {
	validDescriptor := testDescriptor()
	tests := []struct {
		name        string
		descriptor  Descriptor
		errorString string
	}{
		{name: "package", descriptor: Descriptor{Version: validDescriptor.Version, URL: validDescriptor.URL, SHA256: validDescriptor.SHA256}, errorString: "package is required"},
		{name: "version", descriptor: Descriptor{Package: validDescriptor.Package, URL: validDescriptor.URL, SHA256: validDescriptor.SHA256}, errorString: "version is required"},
		{name: "URL", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, SHA256: validDescriptor.SHA256}, errorString: "URL is required"},
		{name: "SHA-256", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, URL: validDescriptor.URL}, errorString: "SHA-256 digest is required"},
		{name: "invalid SHA-256", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, URL: validDescriptor.URL, SHA256: "not-a-digest"}, errorString: "invalid authored-script SHA-256 digest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := NewLocalStore(t.TempDir())
			require.NoError(t, err)

			_, err = store.Open(test.descriptor)

			require.ErrorContains(t, err, test.errorString)
			assert.False(t, errors.Is(err, ErrLocalArtifactNotFound))
		})
	}
}

func TestNilLocalStoreOpen(t *testing.T) {
	var store *LocalStore

	_, err := store.Open(testDescriptor())

	require.ErrorContains(t, err, "local artifact store is not configured")
}
