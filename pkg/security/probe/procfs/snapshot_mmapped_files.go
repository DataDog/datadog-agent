// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// GetMmapedFiles returns the list of executable memory-mapped files for a given process
// Uses the shared GetMappedFiles utility with FilterExecutableRegularFiles
func GetMmapedFiles(p *process.Process) ([]model.SnapshottedMmapedFile, error) {
	// Use shared parsing utilities to get executable regular files (not [vdso], [stack], etc.)
	paths, err := GetMappedFiles(int32(p.Pid), MaxMmapedFilesPerProcess, FilterExecutableRegularFiles)
	if err != nil {
		return nil, err
	}

	// Convert to SnapshottedMmapedFile format
	mmapedFiles := make([]model.SnapshottedMmapedFile, 0, len(paths))
	for _, path := range paths {
		mmapedFiles = append(mmapedFiles, model.SnapshottedMmapedFile{
			Path: path,
		})
	}

	return mmapedFiles, nil
}
