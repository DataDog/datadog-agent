// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procmaps reads the address space of a process through procfs: its
// mappings and its memory. It stays free of agent dependencies so that the
// ptracer, which ships in cws-instrumentation, can link it.
package procmaps

import (
	"bytes"
	"iter"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// MapsEntry represents a parsed entry from /proc/[pid]/maps
type MapsEntry struct {
	StartAddr   uint64
	EndAddr     uint64
	Offset      uint64 // offset into the mapped file
	Permissions string // e.g., "r-xp", "rw-p"
	Pathname    string // e.g., "/usr/lib/libc.so.6" or "[heap]"
}

// ParseMapsLine parses a single line from /proc/[pid]/maps
//
// From `man procfs`: The format of the file is:
//
//	address           perms offset  dev   inode       pathname
//	00400000-00452000 r-xp 00000000 08:02 173521      /usr/bin/dbus-daemon
//	00651000-00652000 r--p 00051000 08:02 173521      /usr/bin/dbus-daemon
//	00652000-00655000 rw-p 00052000 08:02 173521      /usr/bin/dbus-daemon
func ParseMapsLine(line []byte) (MapsEntry, bool) {
	entry := MapsEntry{}
	i := 0
	for field := range fieldsSeqN(line, 6) {
		switch i {
		case 0:
			// Best-effort: a malformed range leaves the addresses zero instead of
			// rejecting the entry, so callers that only need perms/pathname still work.
			address := field
			if dash := bytes.IndexByte(address, '-'); dash > 0 {
				entry.StartAddr, _ = strconv.ParseUint(string(address[:dash]), 16, 64)
				entry.EndAddr, _ = strconv.ParseUint(string(address[dash+1:]), 16, 64)
			}
		case 1:
			entry.Permissions = string(field)
		case 2:
			entry.Offset, _ = strconv.ParseUint(string(field), 16, 64)
		case 5:
			entry.Pathname = string(field)
		}
		i++
	}

	return entry, i > 0
}

var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}

// fieldsSeqN returns an iterator over subslices of s split around runs of
// whitespace characters, as defined by [unicode.IsSpace].
// It returns at most n subslices; the last subslice will be the unsplit remainder.
func fieldsSeqN(s []byte, n int) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		if n <= 0 {
			return
		}
		n = min(n, len(s))
		start := -1
		f := 0
		for i := 0; i < len(s); {
			size := 1
			r := rune(s[i])
			isSpace := asciiSpace[s[i]] != 0
			if r >= utf8.RuneSelf {
				r, size = utf8.DecodeRune(s[i:])
				isSpace = unicode.IsSpace(r)
			}
			if isSpace {
				if start >= 0 {
					if !yield(s[start:i:i]) {
						return
					}
					f++
					start = -1
				}
			} else if start < 0 {
				start = i
				if f == n-1 {
					break
				}
			}
			i += size
		}
		if start >= 0 {
			yield(s[start:len(s):len(s)])
		}
	}
}

// MapsFilterFunc is a function that determines whether a maps entry should be included
type MapsFilterFunc func(entry MapsEntry) bool

// Common filter functions

// FilterReadableOnly returns true for entries with read permission
func FilterReadableOnly(entry MapsEntry) bool {
	return len(entry.Permissions) > 0 && entry.Permissions[0] == 'r'
}

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
