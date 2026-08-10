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

// verifyDirComponent is the pure policy behind the ancestor walk in
// secureRuntimeDir. Exercising it directly covers the accept/reject logic
// regardless of the euid the suite runs under (the filesystem-level test above
// skips when run as root, which is common for eBPF suites).
func TestVerifyDirComponent(t *testing.T) {
	const (
		rootUID    uint32 = 0
		nonRootUID uint32 = 1000
	)
	tests := []struct {
		name    string
		mode    os.FileMode
		uid     uint32
		wantErr bool
	}{
		{"root-owned 0700 dir", os.ModeDir | 0700, rootUID, false},
		{"root-owned 0755 dir", os.ModeDir | 0755, rootUID, false},
		{"root-owned sticky 1777 dir (/var/tmp)", os.ModeDir | os.ModeSticky | 0777, rootUID, false},
		{"symlink", os.ModeSymlink | 0777, rootUID, true},
		{"regular file", 0644, rootUID, true},
		{"non-root-owned dir", os.ModeDir | 0700, nonRootUID, true},
		{"group-writable dir without sticky", os.ModeDir | 0770, rootUID, true},
		{"world-writable dir without sticky", os.ModeDir | 0777, rootUID, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyDirComponent("/some/path", tc.mode, tc.uid)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
