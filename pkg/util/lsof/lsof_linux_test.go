// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestOpenFiles(t *testing.T) {
	log.SetupLogger(seelog.Default, "debug")

	ctx := context.Background()
	pid := 221

	files, err := openFiles(ctx, pid)

	assert.NoError(t, err)
	assert.NotEmpty(t, files)

	t.Logf("%+v\n", files)
}

//TODO: openFiles
//TODO: mmapMetadata
//TODO: fdMetadata
//TODO: fdStat
//TODO: readSocketInfo

func TestPermToString(t *testing.T) {
	testCases := []struct {
		perms    *procfs.ProcMapPermissions
		expected string
	}{
		{
			perms: &procfs.ProcMapPermissions{
				Private: true,
			},
			expected: "---p",
		},
		{
			perms: &procfs.ProcMapPermissions{
				Read:   true,
				Shared: true,
			},
			expected: "r--s",
		},
		{
			perms: &procfs.ProcMapPermissions{
				Write:   true,
				Private: true,
			},
			expected: "-w-p",
		},
		{
			perms: &procfs.ProcMapPermissions{
				Execute: true,
				Shared:  true,
			},
			expected: "--xs",
		},
		{
			perms: &procfs.ProcMapPermissions{
				Read:    true,
				Write:   true,
				Execute: true,
				Shared:  true,
			},
			expected: "rwxs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			res := permToString(tc.perms)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestMmapFD(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		ty       string
		cwd      string
		expected string
	}{
		{
			"regular file",
			"/some/path",
			"REG",
			"/some/cwd",
			"mem",
		},
		{
			"directory",
			"/",
			"DIR",
			"",
			"rtd",
		},
		{
			"cwd",
			"/some/cwd",
			"DIR",
			"/some/cwd",
			"cwd",
		},
		{
			"unknown",
			"/some/path",
			"PIPE",
			"",
			"unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := mmapFD(tc.path, tc.ty, tc.cwd)
			assert.Equal(t, tc.expected, res)
		})
	}
}

type mockFileInfo struct {
	modTime time.Time
	mode    fs.FileMode
	name    string
	size    int64
	sys     any
}

func (m *mockFileInfo) IsDir() bool {
	return m.mode.IsDir()
}
func (m *mockFileInfo) ModTime() time.Time {
	return m.modTime
}
func (m *mockFileInfo) Mode() fs.FileMode {
	return m.mode
}
func (m *mockFileInfo) Name() string {
	return m.name
}
func (m *mockFileInfo) Size() int64 {
	return m.size
}
func (m *mockFileInfo) Sys() any {
	return m.sys
}

func TestFileStats(t *testing.T) {
	testCases := []struct {
		name     string
		fileType os.FileMode
		fileTy   string
		filePerm os.FileMode
		size     int64
		inode    uint64
	}{
		{
			"regular file",
			0,
			"REG",
			0777,
			12,
			42,
		},
		{
			"socket",
			os.ModeSocket,
			"SOCKET",
			0600,
			0,
			123456789,
		},
		{
			"pipe",
			os.ModeNamedPipe,
			"PIPE",
			0220,
			0,
			67890,
		},
		{
			"device",
			os.ModeDevice,
			"DEV",
			0400,
			0,
			78901,
		},
		{
			"dir",
			os.ModeDir,
			"DIR",
			0,
			0,
			42,
		},
		{
			"character device",
			os.ModeCharDevice,
			"CHAR",
			0404,
			0,
			666,
		},
		{
			"symlink",
			os.ModeSymlink,
			"LINK",
			0666,
			8,
			9999,
		},
		{
			"irregular",
			os.ModeIrregular,
			"?",
			0,
			0,
			0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stat := func(path string) (os.FileInfo, error) {
				fi := &mockFileInfo{
					modTime: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
					mode:    tc.fileType | tc.filePerm,
					name:    "somename",
					size:    tc.size,
					sys:     &syscall.Stat_t{Ino: tc.inode},
				}
				return fi, nil
			}

			ty, perm, size, ino := fileStats(stat, "/some/path")
			require.NotEmpty(t, ty)

			assert.Equal(t, tc.fileTy, ty)
			assert.Equal(t, tc.filePerm.String(), perm)
			assert.Equal(t, tc.size, size)
			assert.Equal(t, tc.inode, ino)
		})
	}
}

func TestFileStatsErr(t *testing.T) {
	stat := func(path string) (os.FileInfo, error) {
		return nil, errors.New("some error")
	}
	ty, _, _, _ := fileStats(stat, "/some/path")
	require.Empty(t, ty)
}

func TestFileStatsNoSys(t *testing.T) {
	stat := func(path string) (os.FileInfo, error) {
		return &mockFileInfo{}, nil
	}

	ty, perm, size, ino := fileStats(stat, "/some/path")
	assert.Equal(t, "REG", ty)
	assert.Equal(t, "----------", perm)
	assert.EqualValues(t, 0, size)
	assert.EqualValues(t, 0, ino)
}

func TestProcPath(t *testing.T) {
	assert.Equal(t, "/proc", procPath())

	t.Setenv("HOST_PROC", "/myproc")
	assert.Equal(t, "/myproc", procPath())
}
