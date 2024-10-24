// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AllPidsProcs will return all pids under procRoot
func AllPidsProcs(procRoot string) ([]int, error) {
	f, err := os.Open(procRoot)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dirs, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pids := make([]int, 0, len(dirs))
	for _, name := range dirs {
		if pid, err := strconv.Atoi(name); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// WithAllProcs will execute `fn` for every pid under procRoot. `fn` is
// passed the `pid`. If `fn` returns an error the iteration aborts,
// returning the last error returned from `fn`.
func WithAllProcs(procRoot string, fn func(int) error) error {
	pids, err := AllPidsProcs(procRoot)
	if err != nil {
		return err
	}

	for _, pid := range pids {
		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}

// ProcMapEntry represents an entry of the /proc/X/maps file
type ProcMapEntry struct {
	Start   uint64
	End     uint64
	Mode    string
	Offset  uint64
	MajorID uint64
	MinorID uint64
	Inode   uint64
	Path    string
	Deleted bool
}

// ProcMapEntries is a collection of ProcMapEntry objects sorted by start address
type ProcMapEntries []ProcMapEntry

// Insert will insert the entry in the correct position in the slice based on its start address
func (p *ProcMapEntries) Insert(entry ProcMapEntry) {
	for i, other := range *p {
		if entry.Start < other.Start {
			if i == 0 {
				*p = append([]ProcMapEntry{entry}, *p...)
				return
			}

			*p = append(append((*p)[:i-1], entry), (*p)[i:]...)
			return
		}
	}

	*p = append(*p, entry)
}

// FindEntryForAddress will return the entry that contains the given address
func (p *ProcMapEntries) FindEntryForAddress(addr uint64) *ProcMapEntry {
	// Probably could be made more efficient, but for a first implementation it should be fine
	for i := range *p {
		if addr >= (*p)[i].Start && addr < (*p)[i].End {
			return &(*p)[i]
		}
	}

	return nil
}

// readProcessMemMapsFromBuffer reads the content of a /proc/PID/maps file from a buffer
// Format reference: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/fs/proc/task_mmu.c#n304
// see show_vma_header_prefix function
func readProcessMemMapsFromBuffer(buffer io.Reader) (ProcMapEntries, error) {
	scanner := bufio.NewScanner(buffer)

	var entries ProcMapEntries
	var err error

	// For clarity of the arguments to ParseUint
	const formatHex, formatDec = 16, 10
	const width64, width16 = 64, 16

	for scanner.Scan() {
		var entry ProcMapEntry
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 {
			// Ignore empty lines
			continue
		}

		addrFields := strings.Split(fields[0], "-")
		majminFields := strings.Split(fields[3], ":")
		if len(addrFields) != 2 || len(majminFields) != 2 {
			return nil, fmt.Errorf("invalid line (line=%s)", line)
		}

		entry.Start, err = strconv.ParseUint(addrFields[0], formatHex, width64)
		if err != nil {
			return nil, fmt.Errorf("invalid start address (line=%s)", line)
		}

		entry.End, err = strconv.ParseUint(addrFields[1], formatHex, width64)
		if err != nil {
			return nil, fmt.Errorf("invalid end address (line=%s)", line)
		}

		entry.Mode = fields[1]
		entry.Offset, err = strconv.ParseUint(fields[2], formatHex, width64)
		if err != nil {
			return nil, fmt.Errorf("invalid offset (line=%s)", line)
		}

		entry.MajorID, err = strconv.ParseUint(majminFields[0], formatHex, width16)
		if err != nil {
			return nil, fmt.Errorf("invalid major id (line=%s)", line)
		}

		entry.MinorID, err = strconv.ParseUint(majminFields[1], formatHex, width16)
		if err != nil {
			return nil, fmt.Errorf("invalid minor id (line=%s)", line)
		}

		entry.Inode, err = strconv.ParseUint(fields[4], formatDec, width64)
		if err != nil {
			return nil, fmt.Errorf("invalid inode (line=%s)", line)
		}

		if len(fields) >= 6 {
			entry.Path = fields[5]
		}

		if len(fields) >= 7 && fields[6] == "(deleted)" {
			entry.Deleted = true
		}

		entries.Insert(entry)
	}

	return entries, nil
}

// ReadProcessMemMaps reads the content of a /proc/PID/maps file and returns all the entries found in it
func ReadProcessMemMaps(pid int, procRoot string) (ProcMapEntries, error) {
	mapsPath := filepath.Join(procRoot, strconv.Itoa(pid), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", mapsPath, err)
	}
	defer mapsFile.Close()

	entries, err := readProcessMemMapsFromBuffer(bufio.NewReader(mapsFile))
	if err != nil {
		return nil, fmt.Errorf("cannot read maps from %s: %w", mapsPath, err)
	}

	return entries, nil
}
