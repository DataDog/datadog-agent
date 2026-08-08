// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// secureRuntimeDir must reject a cache directory that is not owned by root,
// since only root should be able to write or read the compiled objects stored
// there.
func TestSecureRuntimeDirRejectsNonRootOwned(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root: a root-created dir would legitimately pass")
	}

	dir := filepath.Join(t.TempDir(), "build")
	err := secureRuntimeDir(dir)
	require.Error(t, err, "a non-root-owned cache directory must be rejected")
}

// secureRuntimeDir must reject a directory whose path traverses a symlinked
// component.
func TestSecureRuntimeDirRejectsSymlinkComponent(t *testing.T) {
	base := t.TempDir()

	realDir := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(realDir, 0700))

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realDir, link))

	err := secureRuntimeDir(filepath.Join(link, "build"))
	require.Error(t, err, "a cache directory reached through a symlink must be rejected")
}
