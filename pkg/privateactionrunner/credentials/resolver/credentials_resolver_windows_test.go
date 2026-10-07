// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package resolver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptCredentialFileResolutionPreventsAllowedRootReplacement(t *testing.T) {
	parent := t.TempDir()
	allowedRoot := filepath.Join(parent, "allowed")
	require.NoError(t, os.Mkdir(allowedRoot, 0o700))
	path := filepath.Join(allowedRoot, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	resolver := newTestResolver(t, []string{allowedRoot})

	require.Error(t, os.Rename(allowedRoot, filepath.Join(parent, "moved")))

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(path), nil)

	require.NoError(t, err)
	assert.Equal(t, "original", credentials.AsTokenMap()["configFileLocation"])
}

func TestScriptCredentialFileResolutionWithForwardSlashPath(t *testing.T) {
	allowedRoot := t.TempDir()
	path := filepath.Join(allowedRoot, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte("credentials"), 0o600))
	resolver := newTestResolver(t, []string{allowedRoot})

	t.Run("accepts regular file", func(t *testing.T) {
		credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(filepath.ToSlash(path)), nil)

		require.NoError(t, err)
		assert.Equal(t, "credentials", credentials.AsTokenMap()["configFileLocation"])
	})

	t.Run("rejects parent traversal", func(t *testing.T) {
		traversalPath := filepath.ToSlash(allowedRoot) + "/nested/../credentials.yaml"
		_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(traversalPath), nil)

		require.ErrorIs(t, err, errCouldNotLoadScriptCredentialFile)
	})
}

func TestFilesystemRootPathWindows(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "drive", path: `C:\Data\Scripts`, want: `C:\`},
		{name: "UNC share", path: `\\server\share\scripts`, want: `\\server\share\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filesystemRootPath(tt.path))
		})
	}
}
