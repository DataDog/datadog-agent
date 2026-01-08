// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openPathWithoutSymlinks opens a file by traversing each path component with
// O_NOFOLLOW, to ensure that the actual filesystem structure matches the path
// passed to the function.  On newer kernels, this could be done with openat2(2)
// and RESOLVE_NO_SYMLINKS.
//
// Note that we can't use google/safeopen, cyphar/filepath-securejoin, nor
// os.OpenInRoot(), since all of those follow symlinks on the root directory
// itself, but for our use case, the root directory itself can not be trusted to
// not have changed since the time the path was validated.
func openPathWithoutSymlinks(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be absolute: %s", path)
	}

	// Split path into components
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))

	dirFd, err := unix.Open("/", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open root directory: %w", err)
	}
	defer func() {
		if dirFd >= 0 {
			unix.Close(dirFd)
		}
	}()

	// Open each directory component with O_NOFOLLOW
	// parts[0] is empty string before the leading /, parts[1..n-1] are directories, parts[n] is file
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}

		newFd, err := unix.Openat(dirFd, parts[i], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to open directory component %s: %w", parts[i], err)
		}

		// Close the previous directory fd
		unix.Close(dirFd)
		dirFd = newFd
	}

	// Open the final file component with O_NOFOLLOW
	fileName := parts[len(parts)-1]
	fileFd, err := unix.Openat(dirFd, fileName, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}

	// Convert fd to *os.File
	return os.NewFile(uintptr(fileFd), path), nil
}
