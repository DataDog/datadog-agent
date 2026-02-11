// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// MaxMmapedFilesPerProcess defines the maximum number of mmaped files per process
const MaxMmapedFilesPerProcess = 128

// MapsEntry represents a parsed entry from /proc/[pid]/maps
type MapsEntry struct {
	Permissions string // e.g., "r-xp", "rw-p"
	Pathname    string // e.g., "/usr/lib/libc.so.6" or "[heap]"
}

var (
	// From `man procfs`: The format of the file is:
	//
	//    address           perms offset  dev   inode       pathname
	//    00400000-00452000 r-xp 00000000 08:02 173521      /usr/bin/dbus-daemon
	//    00651000-00652000 r--p 00051000 08:02 173521      /usr/bin/dbus-daemon
	//    00652000-00655000 rw-p 00052000 08:02 173521      /usr/bin/dbus-daemon
	mapsLineRegex = regexp.MustCompile(`^` +
		`(?:\S+)` + // address
		`\s+(?P<perms>\S+)` + // perms
		`\s+(?:\S+)` + // offset
		`\s+(?:\S+)` + // dev
		`\s+(?:\S+)` + // inode
		`(?:\s+(?P<pathname>.+))?` +
		`$`)
	permsIdx    = mapsLineRegex.SubexpIndex("perms")
	pathnameIdx = mapsLineRegex.SubexpIndex("pathname")
)

// ParseMapsLine parses a single line from /proc/[pid]/maps
func ParseMapsLine(line []byte) (MapsEntry, bool) {
	m := mapsLineRegex.FindSubmatchIndex(line)
	if len(m) == 0 {
		return MapsEntry{}, false
	}

	entry := MapsEntry{}

	// Extract permissions
	if m[permsIdx*2] != -1 {
		entry.Permissions = string(line[m[permsIdx*2]:m[permsIdx*2+1]])
	}

	// Extract pathname
	if m[pathnameIdx*2] != -1 {
		entry.Pathname = string(line[m[pathnameIdx*2]:m[pathnameIdx*2+1]])
	}

	return entry, true
}

// MapsFilterFunc is a function that determines whether a maps entry should be included
type MapsFilterFunc func(entry MapsEntry) bool

// GetMappedFiles reads /proc/[pid]/maps and returns filtered file paths
// Parameters:
//   - pid: process ID to read maps for
//   - maxFiles: maximum number of files to return (0 = unlimited)
//   - filter: optional filter function (nil = include all)
//
// Returns a deduplicated list of file paths matching the filter
func GetMappedFiles(pid int32, maxFiles int, filter MapsFilterFunc) ([]string, error) {
	mapsPath := kernel.HostProc(strconv.Itoa(int(pid)), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return nil, err
	}
	defer mapsFile.Close()

	if maxFiles <= 0 {
		maxFiles = MaxMmapedFilesPerProcess
	}

	files := make([]string, 0, maxFiles)
	seenPaths := make(map[string]struct{})
	scanner := bufio.NewScanner(mapsFile)

	for scanner.Scan() && len(files) < maxFiles {
		entry, ok := ParseMapsLine(scanner.Bytes())
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

// Common filter functions

// FilterExecutableOnly returns true for entries with execute permission
func FilterExecutableOnly(entry MapsEntry) bool {
	return bytes.Contains([]byte(entry.Permissions), []byte("x"))
}

// FilterRegularFiles returns true for entries that are regular files (not special mappings)
func FilterRegularFiles(entry MapsEntry) bool {
	// Skip special mappings like [vdso], [stack], [heap]
	return !bytes.HasPrefix([]byte(entry.Pathname), []byte("["))
}

// FilterExecutableRegularFiles combines executable and regular file filters
func FilterExecutableRegularFiles(entry MapsEntry) bool {
	return FilterExecutableOnly(entry) && FilterRegularFiles(entry)
}

// FilterExcludePath returns a filter that excludes a specific path
func FilterExcludePath(excludePath string) MapsFilterFunc {
	return func(entry MapsEntry) bool {
		return entry.Pathname != excludePath
	}
}

// CombineFilters combines multiple filters with AND logic
func CombineFilters(filters ...MapsFilterFunc) MapsFilterFunc {
	return func(entry MapsEntry) bool {
		for _, f := range filters {
			if !f(entry) {
				return false
			}
		}
		return true
	}
}
