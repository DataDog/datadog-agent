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
	prefix = "socket:["
	// readlinkBufferSize is the minimum size needed to read a socket path
	// which looks like "socket:[165614651]"
	readlinkBufferSize = 64
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

// getSockets get a list of socket inode numbers opened by a process
func getSockets(pid int32, buf []byte) ([]uint64, error) {
	if len(buf) < readlinkBufferSize {
		return nil, io.ErrShortBuffer
	}

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

	dirFd := int(d.Fd())

	var sockets []uint64
	for _, fd := range fnames {
		n, err := readlinkat(dirFd, fd, buf)
		if err != nil {
			continue
		}
		if strings.HasPrefix(string(buf[:n]), prefix) {
			sock, err := strconv.ParseUint(string(buf[len(prefix):n-1]), 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
}
