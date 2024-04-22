// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

var (
	fsDisk = filesystem.NewDisk()
)

var (
	// ErrNotEnoughDiskSpace is returned when there is not enough disk space.
	ErrNotEnoughDiskSpace = errors.New("not enough disk space")
)

// CheckAvailableDiskSpace checks if there is enough disk space at the given paths.
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func CheckAvailableDiskSpace(requiredDiskSpace uint64, paths ...string) error {
	for _, path := range paths {
		_, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("could not stat path %s: %w", path, err)
		}
		s, err := fsDisk.GetUsage(path)
		if err != nil {
			return err
		}
		if s.Available < uint64(requiredDiskSpace) {
			return fmt.Errorf("%w: %d bytes available at %s, %d required", ErrNotEnoughDiskSpace, s.Available, path, requiredDiskSpace)
		}
	}
	return nil
}
