// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procmaps

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxLineBuf bounds a single /proc/pid/maps or /proc/pid/smaps line/record.
// Kernel-generated pathnames are short in practice; this is a defensive
// ceiling against a pathological mount namespace or bind-mount chain
// producing an extremely long pathname field.
const maxLineBuf = 1 << 20

// maxCommBytes is deliberately larger than Linux's TASK_COMM_LEN while still
// bounding reads from a configured proc path that is not a real procfs.
const maxCommBytes = 4096

func readImpl(ctx context.Context, procPath string, pid int, extended bool) (string, []Mapping, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	pidDir := filepath.Join(procPath, strconv.Itoa(pid))

	name, err := readComm(pidDir)
	if err != nil {
		return "", nil, err
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	var mappings []Mapping
	if extended {
		mappings, err = readSmaps(ctx, pidDir)
	} else {
		mappings, err = readMaps(ctx, pidDir)
	}
	if err != nil {
		return "", nil, err
	}
	return name, mappings, nil
}

// readComm reads the short process name from pidDir/comm, translating
// ENOENT into ErrNoSuchProcess so callers can report a consistent message
// regardless of exactly which /proc/pid/* file first fails to open (the
// process may exit between mapping enumeration steps).
func readComm(pidDir string) (string, error) {
	f, err := os.Open(filepath.Join(pidDir, "comm"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoSuchProcess
		}
		return "", fmt.Errorf("read process name: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxCommBytes+1))
	if err != nil {
		return "", fmt.Errorf("read process name: %w", err)
	}
	if len(data) > maxCommBytes {
		return "", ErrMalformedData
	}
	return sanitizeDisplayName(strings.TrimRight(string(data), "\r\n")), nil
}

// readMaps parses /proc/pid/maps into Mapping values without per-mapping
// RSS/Dirty (basic, non-extended mode).
func readMaps(ctx context.Context, pidDir string) ([]Mapping, error) {
	f, err := os.Open(filepath.Join(pidDir, "maps"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoSuchProcess
		}
		return nil, fmt.Errorf("read process memory maps: %w", err)
	}
	defer f.Close()

	var mappings []Mapping
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), maxLineBuf)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m, ok := parseMapsLine(scanner.Text())
		if !ok {
			return nil, ErrMalformedData
		}
		if len(mappings) >= MaxMappings {
			return nil, ErrMappingLimitExceeded
		}
		mappings = append(mappings, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read process memory maps: %w", err)
	}
	return mappings, nil
}

// parseMapsLine parses a single /proc/pid/maps line:
//
//	<start>-<end> <perms> <offset> <dev> <inode>[ <pathname>]
func parseMapsLine(line string) (Mapping, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return Mapping{}, false
	}
	start, end, ok := parseAddrRange(fields[0])
	if !ok || end <= start || len(fields[1]) != 4 {
		return Mapping{}, false
	}
	// The pathname is whatever follows the 5th field in the original line
	// (fields[5:] joined), preserving embedded spaces in rare pathnames.
	pathname := ""
	if idx := fieldsAfter(line, 5); idx >= 0 {
		pathname = strings.TrimSpace(line[idx:])
	}
	return Mapping{
		Start: start,
		End:   end,
		Perms: mapsPermsToMode(fields[1]),
		Name:  mappingName(pathname),
	}, true
}

// fieldsAfter returns the byte offset into line immediately after the n-th
// whitespace-delimited field (1-indexed count of fields consumed), or -1 if
// line has fewer than n fields. Used to recover the raw pathname tail
// (fields 6+) with any embedded spaces intact, which strings.Fields would
// otherwise split apart.
func fieldsAfter(line string, n int) int {
	inField := false
	count := 0
	for i, r := range line {
		if r == ' ' || r == '\t' {
			inField = false
			continue
		}
		if !inField {
			count++
			inField = true
		}
		if count > n {
			return i
		}
	}
	return -1
}

// mapsPermsToMode converts a maps-style 4-char perms field (e.g. "r-xp")
// into pmap's 5-char Mode column: read, write, execute, then 's' for a
// shared mapping or '-' for private, followed by a fixed trailing '-'.
func mapsPermsToMode(perms string) string {
	b := []byte("-----")
	for i := 0; i < len(perms) && i < 4; i++ {
		switch {
		case i == 0 && perms[0] == 'r':
			b[0] = 'r'
		case i == 1 && perms[1] == 'w':
			b[1] = 'w'
		case i == 2 && perms[2] == 'x':
			b[2] = 'x'
		case i == 3 && perms[3] == 's':
			b[3] = 's'
		}
	}
	return string(b)
}

// mappingName maps a raw /proc/pid/maps pathname field to pmap's
// display convention: the file's base name for file-backed mappings,
// the bracketed special name verbatim (e.g. "[heap]", "[stack]"), or
// "[ anon ]" for anonymous private memory with no backing file.
func mappingName(pathname string) string {
	if pathname == "" {
		return "[ anon ]"
	}
	if strings.HasPrefix(pathname, "[") {
		return sanitizeDisplayName(pathname)
	}
	return sanitizeDisplayName(filepath.Base(pathname))
}

// sanitizeDisplayName keeps kernel-controlled process and mapping labels on a
// single printable output line. Linux escapes newlines in maps pathnames, but
// task comm values can contain control bytes.
func sanitizeDisplayName(name string) string {
	b := []byte(name)
	for i := range b {
		if b[i] < ' ' || b[i] == 0x7f {
			b[i] = '?'
		}
	}
	return string(b)
}

// parseAddrRange parses a "<start>-<end>" hex address range.
func parseAddrRange(s string) (start, end uint64, ok bool) {
	dash := strings.IndexByte(s, '-')
	if dash < 0 {
		return 0, 0, false
	}
	start, err := strconv.ParseUint(s[:dash], 16, 64)
	if err != nil {
		return 0, 0, false
	}
	end, err = strconv.ParseUint(s[dash+1:], 16, 64)
	if err != nil {
		return 0, 0, false
	}
	return start, end, true
}

// readSmaps parses /proc/pid/smaps into Mapping values with per-mapping
// RSS and Dirty (Private_Dirty + Shared_Dirty) populated, for extended
// (-x) mode.
func readSmaps(ctx context.Context, pidDir string) ([]Mapping, error) {
	f, err := os.Open(filepath.Join(pidDir, "smaps"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoSuchProcess
		}
		return nil, fmt.Errorf("read extended process memory maps: %w", err)
	}
	defer f.Close()

	var mappings []Mapping
	var cur *Mapping
	var sawRSS, sawPrivateDirty, sawSharedDirty bool
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), maxLineBuf)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scanner.Text()
		if m, ok := parseMapsLine(line); ok {
			if cur != nil && (!sawRSS || !sawPrivateDirty || !sawSharedDirty) {
				return nil, ErrMalformedData
			}
			if len(mappings) >= MaxMappings {
				return nil, ErrMappingLimitExceeded
			}
			m.HasRSS = true
			mappings = append(mappings, m)
			cur = &mappings[len(mappings)-1]
			sawRSS, sawPrivateDirty, sawSharedDirty = false, false, false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.IndexByte(fields[0], '-') >= 0 {
			return nil, ErrMalformedData
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Rss:"):
			if sawRSS {
				return nil, ErrMalformedData
			}
			value, ok := parseSmapsKBField(line)
			if !ok || value > ^uint64(0)-cur.RSSKB {
				return nil, ErrMalformedData
			}
			cur.RSSKB += value
			sawRSS = true
		case strings.HasPrefix(line, "Private_Dirty:"):
			if sawPrivateDirty {
				return nil, ErrMalformedData
			}
			value, ok := parseSmapsKBField(line)
			if !ok || value > ^uint64(0)-cur.DirtyKB {
				return nil, ErrMalformedData
			}
			cur.DirtyKB += value
			sawPrivateDirty = true
		case strings.HasPrefix(line, "Shared_Dirty:"):
			if sawSharedDirty {
				return nil, ErrMalformedData
			}
			value, ok := parseSmapsKBField(line)
			if !ok || value > ^uint64(0)-cur.DirtyKB {
				return nil, ErrMalformedData
			}
			cur.DirtyKB += value
			sawSharedDirty = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read extended process memory maps: %w", err)
	}
	if cur != nil && (!sawRSS || !sawPrivateDirty || !sawSharedDirty) {
		return nil, ErrMalformedData
	}
	return mappings, nil
}

// parseSmapsKBField parses a smaps "Key:      123 kB" line's numeric value.
func parseSmapsKBField(line string) (uint64, bool) {
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[2] != "kB" {
		return 0, false
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
