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
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

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

// MaxMmapedFilesPerProcess defines the maximum number of mmaped files to snapshot per process
const MaxMmapedFilesPerProcess = 128

// GetMmapedFiles returns the list of executable memory-mapped files for a given process
func GetMmapedFiles(p *process.Process) ([]model.SnapshottedMmapedFile, error) {
	mmapedFiles := []model.SnapshottedMmapedFile{}

	// Open the /proc/[pid]/maps file
	mapsPath := kernel.HostProc(strconv.Itoa(int(p.Pid)), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		seclog.Tracef("error while opening maps file (pid: %v): %s", p.Pid, err)
		return nil, err
	}
	defer mapsFile.Close()

	// Track unique paths to avoid duplicates (same file can be mapped multiple times)
	seenPaths := make(map[string]struct{})

	scanner := bufio.NewScanner(mapsFile)
	for scanner.Scan() && len(mmapedFiles) < MaxMmapedFilesPerProcess {
		line := scanner.Bytes()

		// Parse the line to extract permissions and pathname
		m := mapsLineRegex.FindSubmatchIndex(line)
		if len(m) == 0 {
			continue
		}

		// Extract permissions
		if m[permsIdx*2] == -1 {
			continue
		}
		perms := line[m[permsIdx*2]:m[permsIdx*2+1]]

		// Only process executable mappings (r-xp)
		// This filters for dynamic libraries and executable code
		if !bytes.Contains(perms, []byte("x")) {
			continue
		}

		// Extract pathname
		if m[pathnameIdx*2] == -1 {
			continue
		}
		pathname := string(line[m[pathnameIdx*2]:m[pathnameIdx*2+1]])

		// Skip if pathname is empty
		if len(pathname) == 0 {
			continue
		}

		// Skip special mappings like [vdso], [stack], [heap]
		if strings.HasPrefix(pathname, "[") {
			continue
		}

		// Skip if we've already seen this path
		if _, seen := seenPaths[pathname]; seen {
			continue
		}

		seenPaths[pathname] = struct{}{}
		mmapedFiles = append(mmapedFiles, model.SnapshottedMmapedFile{
			Path: pathname,
		})
	}

	if err := scanner.Err(); err != nil {
		seclog.Warnf("error while scanning maps file (pid: %v): %s", p.Pid, err)
		return nil, err
	}

	return mmapedFiles, nil
}
