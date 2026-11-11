// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"bufio"
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs/procmaps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// MaxMmapedFilesPerProcess defines the maximum number of mmaped files per process
const MaxMmapedFilesPerProcess = 1024

// GetMappedFiles reads /proc/[pid]/maps and returns filtered file paths
// Parameters:
//   - pid: process ID to read maps for
//   - maxFiles: maximum number of files to return (0 = MaxMmapedFilesPerProcess)
//   - filter: optional filter function (nil = include all)
//
// Returns a deduplicated list of file paths matching the filter
func GetMappedFiles(pid int32, maxFiles int, filter procmaps.MapsFilterFunc) ([]string, error) {
	mapsPath := kernel.HostProc(strconv.Itoa(int(pid)), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return nil, err
	}
	defer mapsFile.Close()

	return readMappedFiles(mapsFile, maxFiles, filter)
}

func readMappedFiles(r io.Reader, maxFiles int, filter procmaps.MapsFilterFunc) ([]string, error) {
	if maxFiles <= 0 {
		maxFiles = MaxMmapedFilesPerProcess
	}

	var files []string
	seenPaths := make(map[string]struct{})
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 128), bufio.MaxScanTokenSize)

	for scanner.Scan() && len(files) < maxFiles {
		entry, ok := procmaps.ParseMapsLine(scanner.Bytes())
		if !ok || entry.Pathname == "" {
			continue
		}

		// Apply filter if provided
		if filter != nil && !filter(entry) {
			continue
		}

		// Deduplicate
		if _, seen := seenPaths[entry.Pathname]; seen {
			continue
		}

		seenPaths[entry.Pathname] = struct{}{}
		files = append(files, entry.Pathname)
	}

	return files, scanner.Err()
}
