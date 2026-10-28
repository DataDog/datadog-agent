// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package headers

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestDownloadKernelHeaders(t *testing.T) {
	flake.Mark(t) // flaky because it reaches out to real package repos
	ebpftest.LogLevel(t, "debug")
	t.Cleanup(func() { HeaderProvider = nil })

	opts := HeaderOptions{
		DownloadEnabled: true,
		Dirs:            []string{t.TempDir()},
		DownloadDir:     t.TempDir(),

		AptConfigDir:   "/etc/apt",
		YumReposDir:    "/etc/yum.repos.d",
		ZypperReposDir: "/etc/zypp/repos.d",
	}
	// The download directory here is a test-owned temp dir, not a root-owned one, so
	// this test supplies a permissive validator; the validation itself is covered below.
	dirs := GetKernelHeaders(opts, func(string) error { return nil })
	assert.NotZero(t, len(dirs), "expected to find header directories")
	t.Log(dirs)

	result := HeaderProvider.GetResult()
	assert.Equal(t, result.IsSuccess(), true)
}

func TestParseHeaderVersion(t *testing.T) {
	cases := []struct {
		body string
		v    kernel.Version
		err  bool
	}{
		{"#define LINUX_VERSION_CODE 328769", kernel.Version(328769), false},
		{"#define  LINUX_VERSION_CODE		123456", kernel.Version(123456), false},
		{"#define LINUX_VERSION_CODE -1", kernel.Version(0), true},
		{"#define LINUX_VERSION_CODE", kernel.Version(0), true},
		{"", kernel.Version(0), true},
	}

	for _, c := range cases {
		hv, err := parseHeaderVersion(bytes.NewBufferString(c.body))
		if c.err {
			assert.Error(t, err, "expected error parsing of `%s`", c.body)
		} else {
			if assert.NoError(t, err, "parse error of `%s`", c.body) {
				assert.Equal(t, c.v, hv, "version mismatch of `%s`", c.body)
			}
		}
	}
}

func TestInvalidExistingKernelHeaders(t *testing.T) {
	kv := kernel.VersionCode(4, 14, 200)
	headerDirName := fmt.Sprintf("linux-headers-%s", kv)

	t.Run("write perms", func(t *testing.T) {
		tmpDir := t.TempDir()
		sp := filepath.Join(tmpDir, "system-probe")
		err := os.MkdirAll(filepath.Join(sp, headerDirName), 0777)
		require.NoError(t, err)

		// must chmod because umask affects mkdir
		err = os.Chmod(sp, 0777)
		require.NoError(t, err)

		_, err = getSysfsHeaderDirs(tmpDir, kv)
		require.ErrorIs(t, err, errInvalidTempDirectory)
	})
	t.Run("user", func(t *testing.T) {
		if os.Geteuid() != 0 {
			t.Skip("skipping test because not root")
		}

		tmpDir := t.TempDir()
		sp := filepath.Join(tmpDir, "system-probe")
		err := os.MkdirAll(filepath.Join(sp, headerDirName), 0777)
		require.NoError(t, err)

		err = os.Chown(sp, 1, 0)
		require.NoError(t, err)

		_, err = getSysfsHeaderDirs(tmpDir, kv)
		require.ErrorIs(t, err, errInvalidTempDirectory)
	})
	t.Run("group", func(t *testing.T) {
		if os.Geteuid() != 0 {
			t.Skip("skipping test because not root")
		}

		tmpDir := t.TempDir()
		sp := filepath.Join(tmpDir, "system-probe")
		err := os.MkdirAll(filepath.Join(sp, headerDirName), 0777)
		require.NoError(t, err)

		err = os.Chown(sp, 0, 1)
		require.NoError(t, err)

		_, err = getSysfsHeaderDirs(tmpDir, kv)
		require.ErrorIs(t, err, errInvalidTempDirectory)
	})
	t.Run("symlink", func(t *testing.T) {
		tmpDir := t.TempDir()
		dst := filepath.Join(tmpDir, "symdst")
		err := os.MkdirAll(filepath.Join(dst, headerDirName), 0777)
		require.NoError(t, err)

		sp := filepath.Join(tmpDir, "system-probe")
		err = os.Symlink(dst, sp)
		require.NoError(t, err)

		_, err = getSysfsHeaderDirs(tmpDir, kv)
		require.ErrorIs(t, err, errInvalidTempDirectory)
	})
}

// When the download directory cannot be secured, the search must stop before the
// directory is read, deleted in, or downloaded into.
func TestUnsecurableDownloadDirStopsSearch(t *testing.T) {
	ebpftest.LogLevel(t, "debug")
	t.Cleanup(func() { HeaderProvider = nil })

	// A stale tree that getKernelHeaders would otherwise validate and delete: it has no
	// critical headers, which is exactly the condition that triggers the cleanup.
	downloadDir := t.TempDir()
	stale := filepath.Join(downloadDir, "usr/src/linux-headers-"+kernelRelease(t))
	require.NoError(t, os.MkdirAll(stale, 0755))
	canary := filepath.Join(stale, "canary")
	require.NoError(t, os.WriteFile(canary, []byte("must survive"), 0644))

	dirs := GetKernelHeaders(HeaderOptions{
		DownloadEnabled: true,
		Dirs:            []string{t.TempDir()}, // present but invalid: skips the local search
		DownloadDir:     downloadDir,
	}, func(string) error {
		return errors.New("download directory could not be secured")
	})

	assert.Empty(t, dirs)
	assert.Equal(t, validationFailure, HeaderProvider.GetResult())
	assert.FileExists(t, canary, "nothing under an unsecurable download directory may be deleted")
}

// A caller that does not supply a validator must not get the downloaded-header path at
// all, rather than silently getting it unvalidated.
func TestNilValidatorDisablesDownloadedHeaders(t *testing.T) {
	ebpftest.LogLevel(t, "debug")
	t.Cleanup(func() { HeaderProvider = nil })

	dirs := GetKernelHeaders(HeaderOptions{
		DownloadEnabled: true,
		Dirs:            []string{t.TempDir()},
		DownloadDir:     t.TempDir(),
	}, nil)

	assert.Empty(t, dirs)
	assert.Equal(t, validationFailure, HeaderProvider.GetResult())
}

func kernelRelease(t *testing.T) string {
	t.Helper()
	rel, err := kernel.Release()
	require.NoError(t, err)
	return rel
}
