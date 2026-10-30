// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setClangBinPath(t *testing.T, path string) {
	original := clangBinPath
	t.Cleanup(func() { clangBinPath = original })
	clangBinPath = path
}

// copyExecutable copies an existing executable into a fresh temporary directory
// with the given mode, so its ownership is that of the test user.
func copyExecutable(t *testing.T, src string, mode os.FileMode) string {
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	require.NoError(t, os.WriteFile(dst, data, mode))
	// WriteFile applies the umask; set the exact mode under test.
	require.NoError(t, os.Chmod(dst, mode))
	return dst
}

func TestClangRefusesUntrustedBinary(t *testing.T) {
	tests := []struct {
		name string
		// skipAsRoot is set for cases that rely on the test user not being root.
		skipAsRoot bool
		binPath    func(t *testing.T) string
		wantErr    string
	}{
		{
			// Group/other-writable is rejected regardless of which user runs the test.
			name:    "group/other writable",
			binPath: func(t *testing.T) string { return copyExecutable(t, "/bin/true", 0777) },
			wantErr: "incorrect permissions",
		},
		{
			name:       "not owned by root",
			skipAsRoot: true,
			binPath:    func(t *testing.T) string { return copyExecutable(t, "/bin/true", 0755) },
			wantErr:    "incorrect permissions",
		},
		{
			name: "symlink",
			binPath: func(t *testing.T) string {
				link := filepath.Join(t.TempDir(), "clang-bpf")
				require.NoError(t, os.Symlink("/bin/true", link))
				return link
			},
			wantErr: "error opening",
		},
		{
			// Opening a FIFO without a writer would block forever without O_NONBLOCK.
			name: "fifo",
			binPath: func(t *testing.T) string {
				fifo := filepath.Join(t.TempDir(), "clang-bpf")
				require.NoError(t, syscall.Mkfifo(fifo, 0755))
				return fifo
			},
			wantErr: "incorrect permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("files created by root are root-owned")
			}
			setClangBinPath(t, tt.binPath(t))

			err := clang(nil)
			require.Error(t, err)
			assert.ErrorContains(t, err, "refusing to execute clang")
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestClangExecutesVerifiedBinary(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to create a root-owned binary")
	}
	setClangBinPath(t, copyExecutable(t, "/bin/echo", 0755))

	out := &bytes.Buffer{}
	require.NoError(t, clang([]string{"hello"}, WithStdout(out)))
	assert.Equal(t, "hello\n", out.String())
}
