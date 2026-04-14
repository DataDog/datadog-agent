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

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	tracermetadatamodel "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"golang.org/x/sys/unix"
)

const (
	socketPrefix = "socket:["
	// readlinkBufferSize is an arbitrary maximum size for the targets of the
	// symlinks in /proc/PID/fd/ for:
	// - log files
	// - sockets, for example "socket:[165614651]"
	// - tracer memfd files, for example "/memfd:datadog-tracer-info-0e057fe5 (deleted)"
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
	sockets        []uint64
	logs           []fdPath
	tracerMetadata *tracermetadatamodel.TracerMetadata
}

// readTracerMemfd reads tracer metadata and modification time from a memfd
// path. Returns nil metadata if the path cannot be stat'd or the content cannot
// be parsed.
func readTracerMemfd(fdPath string) (*tracermetadatamodel.TracerMetadata, int64) {
	info, err := os.Stat(fdPath)
	if err != nil {
		return nil, 0
	}
	tm, err := tracermetadata.GetTracerMetadataFromPath(fdPath)
	if err != nil {
		return nil, 0
	}
	return &tm, info.ModTime().UnixNano()
}

// newestTracerMetadata returns the newest metadata from two candidates,
// comparing by mtime first and using runtime_id as a tie-breaker so that both
// Go and Rust implementations select the same metadata regardless of
// /proc/pid/fd iteration order.
func newestTracerMetadata(a *tracermetadatamodel.TracerMetadata, aTime int64, b *tracermetadatamodel.TracerMetadata, bTime int64) (*tracermetadatamodel.TracerMetadata, int64) {
	if a == nil {
		return b, bTime
	}
	if b == nil {
		return a, aTime
	}
	if bTime > aTime || (bTime == aTime && b.RuntimeID >= a.RuntimeID) {
		return b, bTime
	}
	return a, aTime
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
	pidStr := strconv.Itoa(int(pid))
	var newestTime int64

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

		if tracermetadata.IsTracerMemfdPath(path) {
			memfdPath := kernel.HostProc(pidStr, "fd", fd)
			meta, mtime := readTracerMemfd(memfdPath)
			openFiles.tracerMetadata, newestTime = newestTracerMetadata(openFiles.tracerMetadata, newestTime, meta, mtime)
			continue
		}
	}

	return openFiles, nil
}
