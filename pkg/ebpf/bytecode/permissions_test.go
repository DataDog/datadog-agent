// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package bytecode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// VerifyAssetPermissionsAndOpen opens with O_NOFOLLOW, so a symlink at the final
// path component must be rejected rather than followed to its target.
func TestVerifyAssetPermissionsAndOpenRejectsSymlink(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0644))

	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	f, err := VerifyAssetPermissionsAndOpen(link)
	if f != nil {
		f.Close()
	}
	require.Error(t, err, "opening a symlink must fail with O_NOFOLLOW")
}

// A regular file that is not owned by root must be rejected by the permission
// check even though it opens successfully.
func TestVerifyAssetPermissionsAndOpenRejectsNonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root to observe the ownership rejection")
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "asset.o")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0644))

	f, err := VerifyAssetPermissionsAndOpen(file)
	if f != nil {
		f.Close()
	}
	assert.Error(t, err, "a non-root-owned asset must be rejected")
}

// verifyOwnerPermissions is the pure policy behind the descriptor check above.
// Exercising it directly covers the ownership/permission rejection logic
// regardless of the euid the suite runs under (the filesystem-level test above
// skips when run as root).
func TestVerifyOwnerPermissions(t *testing.T) {
	tests := []struct {
		name    string
		uid     uint32
		gid     uint32
		perm    os.FileMode
		wantErr bool
	}{
		{"root-owned 0644", 0, 0, 0644, false},
		{"root-owned 0600", 0, 0, 0600, false},
		{"root-owned 0755", 0, 0, 0755, false},
		{"non-root uid", 1000, 0, 0644, true},
		{"non-root gid", 0, 1000, 0644, true},
		{"group-writable", 0, 0, 0664, true},
		{"other-writable", 0, 0, 0646, true},
		{"world-writable", 0, 0, 0666, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyOwnerPermissions("asset.o", tc.uid, tc.gid, tc.perm)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
