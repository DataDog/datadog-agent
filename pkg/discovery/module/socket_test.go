// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

func getSocketsOriginal(p *process.Process) ([]uint64, error) {
	FDs, err := p.OpenFiles()
	if err != nil {
		return nil, err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, socketPrefix) {
			inodeStr := strings.TrimPrefix(fd.Path[:len(fd.Path)-1], socketPrefix)
			sock, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
}

func getSocketsOld(pid int32) ([]uint64, error) {
	statPath := kernel.HostProc(fmt.Sprintf("%d/fd", pid))
	d, err := os.Open(statPath)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	fnames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	var sockets []uint64
	for _, fd := range fnames {
		fullPath, err := os.Readlink(filepath.Join(statPath, fd))
		if err != nil {
			continue
		}
		if strings.HasPrefix(fullPath, socketPrefix) {
			sock, err := strconv.ParseUint(fullPath[len(socketPrefix):len(fullPath)-1], 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
}

const (
	numberFDs = 100
)

func createFilesAndSockets(tb testing.TB) {
	listeningSockets := make([]net.Listener, 0, numberFDs)
	tb.Cleanup(func() {
		for _, l := range listeningSockets {
			l.Close()
		}
	})
	for i := 0; i < numberFDs; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(tb, err)
		listeningSockets = append(listeningSockets, l)
	}
	regularFDs := make([]*os.File, 0, numberFDs)
	tb.Cleanup(func() {
		for _, f := range regularFDs {
			f.Close()
		}
	})
	for i := 0; i < numberFDs; i++ {
		f, err := os.CreateTemp("", "")
		require.NoError(tb, err)
		regularFDs = append(regularFDs, f)
	}
}

func TestGetSockets(t *testing.T) {
	createFilesAndSockets(t)
	p, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)

	buf := make([]byte, readlinkBufferSize)
	openFiles, err := getOpenFilesInfo(p.Pid, buf)
	require.NoError(t, err)

	socketsOld, err := getSocketsOld(p.Pid)
	require.NoError(t, err)

	socketsOriginal, err := getSocketsOriginal(p)
	require.NoError(t, err)

	require.Equal(t, openFiles.sockets, socketsOld)
	require.Equal(t, openFiles.sockets, socketsOriginal)
}

func TestGetSocketsBufferTooSmall(t *testing.T) {
	p, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)

	buf := make([]byte, readlinkBufferSize-1)
	_, err = getOpenFilesInfo(p.Pid, buf)
	require.ErrorIs(t, err, io.ErrShortBuffer)
}

func TestReadlinkatBufferFull(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a symlink with a path that is longer than our buffer size
	target := strings.Repeat("A", readlinkBufferSize+1)
	symlinkPath := filepath.Join(tmpDir, "testlink")
	err := os.Symlink(target, symlinkPath)
	if err != nil {
		// Fails on some distros, so skip the test there
		t.Skipf("skipping test due to filename length limit: %v", err)
	}
	require.NoError(t, err)

	dir, err := os.Open(tmpDir)
	require.NoError(t, err)
	defer dir.Close()

	buf := make([]byte, readlinkBufferSize)
	_, err = readlinkat(int(dir.Fd()), "testlink", buf)
	require.ErrorIs(t, err, io.ErrShortBuffer)
}

func BenchmarkGetSockets(b *testing.B) {
	createFilesAndSockets(b)
	buf := make([]byte, readlinkBufferSize)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getOpenFilesInfo(int32(os.Getpid()), buf)
	}
}

func BenchmarkGetSocketsOld(b *testing.B) {
	createFilesAndSockets(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getSocketsOld(int32(os.Getpid()))
	}
}

func BenchmarkGetSocketsOriginal(b *testing.B) {
	createFilesAndSockets(b)
	p, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getSocketsOriginal(p)
	}
}
