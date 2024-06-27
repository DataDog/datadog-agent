// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package replay

import (
	"os"

	"golang.org/x/sys/unix"
)

// getFileContent returns a slice of bytes with the contents of the file specified in the path.
// The mmap flag will try to MMap file so as to achieve reasonable performance with very large
// files while not loading the entire thing into memory.
func getFileContent(path string, mmap bool) ([]byte, error) {

	if !mmap {
		return os.ReadFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := int(stat.Size())

	return unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
}

func unmapFile(b []byte) error {
	return unix.Munmap(b)
}
