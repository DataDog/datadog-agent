// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bufio"
	"fmt"
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

type ProcMapEntry struct {
	Start   uint64
	End     uint64
	Mode    string
	Offset  uint64
	MajorId uint64
	MinorId uint64
	Inode   uint64
	Path    string
	Deleted bool
}

type ProcMapEntries struct {
	Entries []ProcMapEntry
}

func (p *ProcMapEntries) Insert(entry ProcMapEntry) {
	for i, other := range p.Entries {
		if entry.End < other.Start {
			p.Entries = append(append(p.Entries[:i], entry), p.Entries[i:]...)
			return
		}
	}

	p.Entries = append(p.Entries, entry)
}

func (p *ProcMapEntries) FindEntryForAddress(addr uint64) *ProcMapEntry {
	// Probably could be made more efficient, but for a first implementation it should be fine
	for i := range p.Entries {
		if addr >= p.Entries[i].Start && addr < p.Entries[i].End {
			return &p.Entries[i]
		}
	}

	return nil
}

func ReadProcessMemMaps(pid int, procRoot string) (ProcMapEntries, error) {
	mapsPath := filepath.Join(procRoot, strconv.Itoa(pid), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return ProcMapEntries{}, fmt.Errorf("cannot open %s: %w", mapsPath, err)
	}
	defer mapsFile.Close()

	var entries ProcMapEntries
	scanner := bufio.NewScanner(bufio.NewReader(mapsFile))
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
			return ProcMapEntries{}, fmt.Errorf("invalid line in %s: %s", mapsPath, line)
		}

		entry.Start, err = strconv.ParseUint(addrFields[0], 16, 64)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid start address in %s: %s", mapsPath, line)
		}

		entry.End, err = strconv.ParseUint(addrFields[1], 16, 64)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid end address in %s: %s", mapsPath, line)
		}

		entry.Mode = fields[1]
		entry.Offset, err = strconv.ParseUint(fields[2], 16, 64)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid offset in %s: %s", mapsPath, line)
		}

		entry.MajorId, err = strconv.ParseUint(majminFields[0], 10, 16)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid major id in %s: %s", mapsPath, line)
		}

		entry.MinorId, err = strconv.ParseUint(majminFields[1], 10, 16)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid minor id in %s: %s", mapsPath, line)
		}

		entry.Inode, err = strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			return ProcMapEntries{}, fmt.Errorf("invalid inode in %s: %s", mapsPath, line)
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
