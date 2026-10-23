// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package resolver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptCredentialFileResolutionRejectsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files regardless of mode")
	}

	allowedRoot := t.TempDir()
	path := filepath.Join(allowedRoot, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o600))
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	resolver := newTestResolver(t, []string{allowedRoot})
	_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(path), nil)

	require.Error(t, err)
	assert.Equal(t, "could not load script credential file", err.Error())
	assert.NotContains(t, err.Error(), path)
}
