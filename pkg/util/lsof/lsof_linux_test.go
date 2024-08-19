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
	pid := os.Getpid()

	files, err := openFiles(ctx, pid)

	assert.NoError(t, err)
	assert.NotEmpty(t, files)

	t.Logf("%+v\n", files)
}

func TestMmapMetadata(t *testing.T) {
	fs, err := procfs.NewFS("testdata/mmapMetadata")
	require.NoError(t, err)

	proc, err := fs.Proc(1)
	require.NoError(t, err)

	ofl := &openFilesLister{
		pid:      1,
		procPath: "testdata",
		proc:     proc,

		readlink: func(path string) (string, error) {
			return "/some/file", nil
		},
		lstat: func(path string) (os.FileInfo, error) {
			return &mockFileInfo{
				mode: os.ModeSymlink | 0500,
				name: "10",
				size: 16,
				sys:  &syscall.Stat_t{Ino: 123},
			}, nil
		},
		stat: func(path string) (os.FileInfo, error) {
			if path == "/usr/lib/aarch64-linux-gnu/libgcc_s.so.1" {
				return nil, errors.New("some error")
			}
			return &mockFileInfo{
				mode: 0400,
				name: "file",
				size: 8,
				sys:  &syscall.Stat_t{Ino: 456},
			}, nil
		},
	}

	mmaps, err := ofl.mmapMetadata()
	require.NoError(t, err)

	expected := Files{
		{"mem", "REG", "r-xp", "-r--------", 8, "/vscode/vscode-server/bin/linux-arm64/eaa41d57266683296de7d118f574d0c2652e1fc4/node"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libutil.so.1"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/vscode/vscode-server/bin/linux-arm64/eaa41d57266683296de7d118f574d0c2652e1fc4/node_modules/node-pty/build/Release/pty.node"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libc.so.6"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libstdc++.so.6.0.30"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libpthread.so.0"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libm.so.6"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libdl.so.2"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1"},
		{"mem", "REG", "r--p", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1"},
	}

	require.Equal(t, expected, mmaps)
}

func TestMmapMetadataError(t *testing.T) {
	ofl := &openFilesLister{
		proc: &procMock{},
	}

	_, err := ofl.mmapMetadata()
	require.Error(t, err)
}

type procMock struct {
	fileDescriptors []uintptr
	procMaps        []*procfs.ProcMap
	cwd             string
}

func (p *procMock) ProcMaps() ([]*procfs.ProcMap, error) {
	if p.procMaps == nil {
		return nil, errors.New("no proc maps")
	}
	return p.procMaps, nil
}

func (p *procMock) FileDescriptors() ([]uintptr, error) {
	if p.fileDescriptors == nil {
		return nil, errors.New("no file descriptors")
	}
	return p.fileDescriptors, nil
}

func (p *procMock) Cwd() (string, error) {
	if p.cwd == "" {
		return "", errors.New("no cwd")
	}
	return p.cwd, nil
}

func TestFdMetadata(t *testing.T) {
	ofl := &openFilesLister{
		pid:      123,
		procPath: "/myproc",

		proc: &procMock{
			fileDescriptors: []uintptr{3, 4, 5},
		},

		lstat: func(path string) (os.FileInfo, error) {
			if path == "/myproc/123/fd/4" {
				return nil, errors.New("some error")
			}
			return &mockFileInfo{
				mode: os.ModeSymlink | 0500,
				name: "10",
				size: 8,
				sys:  &syscall.Stat_t{Ino: 123},
			}, nil
		},
		stat: func(path string) (os.FileInfo, error) {
			return &mockFileInfo{
				mode: 0400,
				name: "file",
				size: 0,
				sys:  &syscall.Stat_t{Ino: 456},
			}, nil
		},
		readlink: func(path string) (string, error) {
			return "/some/file", nil
		},
		socketInfo: map[uint64]socketInfo{},
	}

	files, err := ofl.fdMetadata()
	require.NoError(t, err)

	expected := Files{
		{
			Fd:       "3",
			Type:     "REG",
			OpenPerm: "-r-x------",
			FilePerm: "-r--------",
			Size:     0,
			Name:     "/some/file",
		},
		{
			Fd:       "5",
			Type:     "REG",
			OpenPerm: "-r-x------",
			FilePerm: "-r--------",
			Size:     0,
			Name:     "/some/file",
		},
	}

	require.ElementsMatch(t, expected, files)
}

func TestFdMetadataError(t *testing.T) {
	ofl := &openFilesLister{
		proc: &procMock{},
	}

	_, err := ofl.fdMetadata()
	require.Error(t, err)
}

func TestFDStat(t *testing.T) {
	testCases := []struct {
		name       string
		fd         uintptr
		lstat      func(string) (os.FileInfo, error)
		stat       func(string) (os.FileInfo, error)
		readlink   func(string) (string, error)
		socketInfo map[uint64]socketInfo
		expected   *File
	}{
		{
			"success socket",
			3,
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSymlink | 0700,
					name: "3",
					size: 8,
					sys:  &syscall.Stat_t{Ino: 123},
				}, nil
			},
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSocket | 0600,
					name: "socket[456]",
					size: 0,
					sys:  &syscall.Stat_t{Ino: 456},
				}, nil
			},
			func(path string) (string, error) {
				return "socket", nil
			},
			map[uint64]socketInfo{
				456: {"127.0.0.1:42->127.0.0.1:43", "connected", "tcp"},
			},
			&File{
				Fd:       "3",
				Type:     "tcp",
				FilePerm: "connected",
				OpenPerm: "-rwx------",
				Size:     0,
				Name:     "127.0.0.1:42->127.0.0.1:43",
			},
		},
		{
			"success regular",
			4,
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSymlink | 0500,
					name: "4",
					size: 8,
					sys:  &syscall.Stat_t{Ino: 124},
				}, nil
			},
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: 0400,
					name: "filename",
					size: 34567890,
					sys:  &syscall.Stat_t{Ino: 789},
				}, nil
			},
			func(path string) (string, error) {
				return "/some/filename", nil
			},
			map[uint64]socketInfo{},
			&File{
				Fd:       "4",
				Type:     "REG",
				FilePerm: "-r--------",
				OpenPerm: "-r-x------",
				Size:     34567890,
				Name:     "/some/filename",
			},
		},
		{
			"error lstat",
			5,
			func(path string) (os.FileInfo, error) {
				return nil, errors.New("some error")
			},
			func(s string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSymlink | 0500,
					name: "4",
					size: 8,
					sys:  &syscall.Stat_t{Ino: 124},
				}, nil
			},
			func(path string) (string, error) {
				return "/some/filename", nil
			},
			map[uint64]socketInfo{},
			nil,
		},
		{
			"error stat",
			6,
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSymlink | 0500,
					name: "6",
					size: 8,
					sys:  &syscall.Stat_t{Ino: 126},
				}, nil
			},
			func(s string) (os.FileInfo, error) {
				return nil, errors.New("some error")
			},
			func(path string) (string, error) {
				return "/some/filename", nil
			},
			map[uint64]socketInfo{},
			nil,
		},
		{
			"error readlink",
			7,
			func(path string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: os.ModeSymlink | 0500,
					name: "7",
					size: 8,
					sys:  &syscall.Stat_t{Ino: 127},
				}, nil
			},
			func(s string) (os.FileInfo, error) {
				return &mockFileInfo{
					mode: 0400,
					name: "filename",
					size: 34567890,
					sys:  &syscall.Stat_t{Ino: 789},
				}, nil
			},
			func(path string) (string, error) {
				return "", errors.New("some error")
			},
			map[uint64]socketInfo{},
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ofl := &openFilesLister{
				lstat:      tc.lstat,
				stat:       tc.stat,
				readlink:   tc.readlink,
				socketInfo: tc.socketInfo,
			}

			file, ok := ofl.fdStat(tc.fd)
			if tc.expected == nil {
				assert.False(t, ok)
			} else {
				require.True(t, ok)
				assert.Equal(t, *tc.expected, file)
			}
		})
	}
}

func TestReadSocketInfo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		info := readSocketInfo("testdata/readSocketInfo/1")

		expected := map[uint64]socketInfo{
			10975:   {"0.0.0.0:18777->0.0.0.0:0", "7", "udp6"},
			40124:   {"10.254.219.58:123->0.0.0.0:0", "7", "udp"},
			1986475: {"127.0.0.1:38489->0.0.0.0:0", "10", "tcp"},
			1987112: {"stream:/tmp/.X11-unix/X2", "unconnected:listen", "unix"},
			2506353: {"stream:", "connected:default", "unix"},
			3359554: {"172.17.0.2:44594->20.199.39.224:443", "1", "tcp6"},
		}

		require.Equal(t, expected, info)
	})

	t.Run("empty", func(t *testing.T) {
		assert.Empty(t, readSocketInfo("testdata/readSocketInfo/2"))
	})

	t.Run("does not exist", func(t *testing.T) {
		assert.Empty(t, readSocketInfo("testdata/readSocketInfo/3"))
	})
}

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
