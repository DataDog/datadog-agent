// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/twmb/murmur3"
	"golang.org/x/sys/unix"
)

// PathIdentifier is the unique key (system wide) of a file based on dev/inode
type PathIdentifier struct {
	Dev   uint64
	Inode uint64
}

type pathIdentifierSet = map[PathIdentifier]struct{}

func (p *PathIdentifier) String() string {
	return fmt.Sprintf("dev/inode %d.%d/%d", unix.Major(p.Dev), unix.Minor(p.Dev), p.Inode)
}

// Key is a unique (system wide) TLDR Base64(murmur3.Sum64(device, inode))
// It composes based the device (minor, major) and inode of a file
// murmur is a non-crypto hashing
//
//	As multiple containers overlayfs (same inode but could be overwritten with different binary)
//	device would be different
//
// a Base64 string representation is returned and could be used in a file path
func (p *PathIdentifier) Key() string {
	buffer := make([]byte, 16)
	binary.LittleEndian.PutUint64(buffer, p.Dev)
	binary.LittleEndian.PutUint64(buffer[8:], p.Inode)
	m := murmur3.Sum64(buffer)
	bufferSum := make([]byte, 8)
	binary.LittleEndian.PutUint64(bufferSum, m)
	// avoid '/' in filename used later by ebpf-manager to register uprobe
	return strings.ReplaceAll(base64.StdEncoding.EncodeToString(bufferSum), "/", "@")
}

// NewPathIdentifier returns a new PathIdentifier instance
// Note that `path` must be an absolute path
func NewPathIdentifier(path string) (pi PathIdentifier, err error) {
	if len(path) < 1 || path[0] != '/' {
		return pi, fmt.Errorf("invalid path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return pi, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return pi, fmt.Errorf("invalid file %q stat %T", path, info.Sys())
	}

	return PathIdentifier{
		Dev:   stat.Dev,
		Inode: stat.Ino,
	}, nil
}
