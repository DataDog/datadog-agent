// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func requireDirectIOTestFile(t *testing.T, name string, content []byte) string {
	t.Helper()

	var refusals []string
	for _, dir := range directIOCandidateDirs(t) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, content, 0600))

		file, err := openDirect(path)
		if err == nil {
			require.NoError(t, file.Close())
			return path
		}
		require.True(t, errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP),
			"unexpected error opening %s with O_DIRECT: %v", path, err)
		refusals = append(refusals, fmt.Sprintf("%s (%v)", dir, err))
	}

	message := "no candidate filesystem supports O_DIRECT: " + strings.Join(refusals, ", ")
	if os.Getenv("DD_REQUIRE_O_DIRECT_TESTS") != "" {
		t.Fatal(message)
	}
	t.Skip(message)
	return ""
}

func directIOCandidateDirs(t *testing.T) []string {
	t.Helper()
	dirs := []string{t.TempDir()}
	if dir, err := os.MkdirTemp("/var/tmp", "dd-directio"); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		dirs = append(dirs, dir)
	}
	return dirs
}

func TestReadDirectFingerprintRangeWithDirect(t *testing.T) {
	content := make([]byte, directIOAlignment*2+211)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := requireDirectIOTestFile(t, "direct.log", content)
	opener := NewFileOpener()
	flags := []types.FileOpenFlag{types.FileOpenFlagDirect}

	tests := []struct {
		name  string
		count int
		want  []byte
	}{
		{name: "bounded range", count: 2061, want: content[:2061]},
		{name: "sub-block range", count: 512, want: content[:512]},
		{name: "range running to EOF", count: len(content), want: content},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := opener.ReadDirectFingerprintRange(path, tt.count, flags)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReadDirectFingerprintRangeReportsPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses DAC permission checks, so O_DIRECT open would not hit EACCES")
	}
	path := filepath.Join(t.TempDir(), "noperm.log")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o000))

	_, err := NewFileOpener().ReadDirectFingerprintRange(path, 4, []types.FileOpenFlag{types.FileOpenFlagDirect})
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestDirectFingerprintReadRequiresSupportedFlags(t *testing.T) {
	path := requireDirectIOTestFile(t, "flags.log", []byte("data"))
	opener := NewFileOpener()

	_, err := opener.ReadDirectFingerprintRange(path, 4, nil)
	require.ErrorContains(t, err, "no supported open flags")
}
