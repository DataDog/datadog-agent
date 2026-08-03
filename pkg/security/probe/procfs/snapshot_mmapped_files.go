// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// MmapedFile represents a snapshotted memory-mapped file
type MmapedFile struct {
	Path string
	model.FileFields
}

// GetMmapedFiles returns the list of executable memory-mapped files for a given process
// Uses the shared GetMappedFiles utility with FilterExecutableRegularFiles
func GetMmapedFiles(pid uint32) ([]MmapedFile, error) {
	// Use shared parsing utilities to get executable regular files (not [vdso], [stack], etc.)
	paths, err := GetMappedFiles(int32(pid), MaxMmapedFilesPerProcess, FilterExecutableRegularFiles)
	if err != nil {
		return nil, err
	}

	mmapedFiles := make([]MmapedFile, 0, len(paths))
	for _, path := range paths {
		mmapedFiles = append(mmapedFiles, MmapedFile{
			Path:       path,
			FileFields: statMmapedFile(pid, path),
		})
	}

	return mmapedFiles, nil
}

// statMmapedFile returns the metadata of a memory-mapped file. It has to be
// collected along with the snapshot: the file is resolved through the root of
// the mapping process, which is gone as soon as that process exits. Best
// effort, zero values are returned for a file that cannot be stat'ed.
func statMmapedFile(pid uint32, path string) model.FileFields {
	var fileStats unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, utils.ProcRootFilePath(pid, path), 0, unix.STATX_ALL, &fileStats); err != nil {
		return model.FileFields{}
	}

	fileFields := model.FileFields{
		Mode:   fileStats.Mode,
		UID:    fileStats.Uid,
		GID:    fileStats.Gid,
		CTime:  uint64(time.Unix(fileStats.Ctime.Sec, int64(fileStats.Ctime.Nsec)).Nanosecond()),
		MTime:  uint64(time.Unix(fileStats.Mtime.Sec, int64(fileStats.Mtime.Nsec)).Nanosecond()),
		Device: fileStats.Dev_major<<20 | fileStats.Dev_minor,
		NLink:  fileStats.Nlink,
	}
	fileFields.Inode = fileStats.Ino
	fileFields.MountID = uint32(fileStats.Mnt_id)

	return fileFields
}
