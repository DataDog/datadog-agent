// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Copyright 2009 The Go Authors. All rights reserved.

//go:build linux

package procutil

import (
	"bytes"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/native"
)

var (
	dotBytes       = []byte(".")
	doubleDotBytes = []byte("..")
)

// countDirent parses directory entries in buf and return the number of entries
func countDirent(buf []byte) (consumed int, count int) {
	origlen := len(buf)
	count = 0
	for len(buf) > 0 {
		reclen, ok := direntReclen(buf)
		if !ok || reclen > uint64(len(buf)) {
			return origlen, count
		}

		rec := buf[:reclen]
		buf = buf[reclen:]

		ino, ok := direntIno(rec)
		if !ok {
			break
		}
		if ino == 0 { // File absent in directory.
			continue
		}
		const namoff = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))
		namlen, ok := direntNamlen(rec)
		if !ok || namoff+namlen > uint64(len(rec)) {
			break
		}
		name := rec[namoff : namoff+namlen]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}

		// skip "." and ".." directories
		if bytes.Equal(name, dotBytes) || bytes.Equal(name, doubleDotBytes) {
			continue
		}
		count++
	}

	return origlen - len(buf), count
}

func direntReclen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Reclen), unsafe.Sizeof(syscall.Dirent{}.Reclen))
}

func direntIno(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Ino), unsafe.Sizeof(syscall.Dirent{}.Ino))
}

func direntNamlen(buf []byte) (uint64, bool) {
	reclen, ok := direntReclen(buf)
	if !ok {
		return 0, false
	}
	return reclen - uint64(unsafe.Offsetof(syscall.Dirent{}.Name)), true
}

// readInt returns the size-bytes unsigned integer in native byte order at offset off.
func readInt(b []byte, off, size uintptr) (u uint64, ok bool) {
	if len(b) < int(off+size) {
		return 0, false
	}

	switch size {
	case 1:
		return uint64(b[off]), true
	case 2:
		return uint64(native.Endian.Uint16(b[off:])), true
	case 4:
		return uint64(native.Endian.Uint32(b[off:])), true
	case 8:
		return native.Endian.Uint64(b[off:]), true
	default:
		panic("syscall: readInt with unsupported size")
	}
}
