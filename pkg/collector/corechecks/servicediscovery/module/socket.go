// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"golang.org/x/sys/unix"
)

const (
	socketPrefix = "socket:["
	// readlinkBufferSize is an arbitrary maximum size for log files (and
	// sockets, but those look like "socket:[165614651]" and are much smaller)
	readlinkBufferSize = 1024
)

func readlinkat(dirfd int, fd string, buf []byte) (int, error) {
	var n int
	var err error
	for {
		n, err = unix.Readlinkat(dirfd, fd, buf)
		if err != unix.EAGAIN {
			break
		}
	}
	if n == len(buf) {
		// This indicates that the buffers was _possibly_ too small to read the
		// entire path. Since we do not expect socket paths to be larger than
		// our buffer size, we just return an error and ignore this file.
		return n, io.ErrShortBuffer
	}
	return n, err
}

type fdPath struct {
	fd   string
	path string
}

type openFilesInfo struct {
	sockets []uint64
	logs    []fdPath
}

// getOpenFilesInfo gets a list of socket inode numbers opened by a process
func getOpenFilesInfo(pid int32, buf []byte) (openFilesInfo, error) {
	openFiles := openFilesInfo{}
	if len(buf) < readlinkBufferSize {
		return openFiles, io.ErrShortBuffer
	}

	statPath := kernel.HostProc(fmt.Sprintf("%d/fd", pid))
	d, err := os.Open(statPath)
	if err != nil {
		return openFiles, err
	}
	defer d.Close()
	fnames, err := d.Readdirnames(-1)
	if err != nil {
		return openFiles, err
	}

	dirFd := int(d.Fd())

	for _, fd := range fnames {
		n, err := readlinkat(dirFd, fd, buf)
		if err != nil {
			continue
		}
		path := string(buf[:n])

		if strings.HasPrefix(path, socketPrefix) {
			sock, err := strconv.ParseUint(path[len(socketPrefix):n-1], 10, 64)
			if err != nil {
				continue
			}
			openFiles.sockets = append(openFiles.sockets, sock)
			continue
		}

		if isLogFile(path) {
			openFiles.logs = append(openFiles.logs, fdPath{fd: fd, path: path})
			continue
		}

		continue
	}

	return openFiles, nil
}
