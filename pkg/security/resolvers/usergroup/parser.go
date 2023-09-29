// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file of the Go repository.

// The code that follows is mostly copied from src/os/user/lookup_unix.go

// Package usergroup holds usergroup related files
package usergroup

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"strconv"
	"strings"
)

var colon = []byte{':'}

// lineFunc returns a value or nil to skip the row.
type lineFunc func(line []byte) any

// readColonFile parses r as an /etc/group or /etc/passwd style file, running
// fn for each row. readColonFile returns a value, an error, or (nil, nil) if
// the end of the file is reached without a match.
//
// readColonFile is the minimum number of colon-separated fields that will be passed
// to fn; in a long line additional fields may be silently discarded.
func readColonFile(r io.Reader, fn lineFunc, readCols int) error {
	rd := bufio.NewReader(r)

	// Read the file line-by-line.
	for {
		var err error
		var isPrefix bool
		var wholeLine []byte

		// Read the next line. We do so in chunks (as much as reader's
		// buffer is able to keep), check if we read enough columns
		// already on each step and store final result in wholeLine.
		for {
			var line []byte
			line, isPrefix, err = rd.ReadLine()

			if err != nil {
				// We should return (nil, nil) if EOF is reached
				// without a match.
				if err == io.EOF {
					err = nil
				}
				return err
			}

			// Simple common case: line is short enough to fit in a
			// single reader's buffer.
			if !isPrefix && len(wholeLine) == 0 {
				wholeLine = line
				break
			}

			wholeLine = append(wholeLine, line...)

			// Check if we read the whole line (or enough columns)
			// already.
			if !isPrefix || bytes.Count(wholeLine, []byte{':'}) >= readCols {
				break
			}
		}

		// There's no spec for /etc/passwd or /etc/group, but we try to follow
		// the same rules as the glibc parser, which allows comments and blank
		// space at the beginning of a line.
		wholeLine = bytes.TrimSpace(wholeLine)
		if len(wholeLine) == 0 || wholeLine[0] == '#' {
			continue
		}

		if fn(wholeLine) == nil {
			continue
		}

		// If necessary, skip the rest of the line
		for ; isPrefix; _, isPrefix, err = rd.ReadLine() {
			if err != nil {
				// We should return (nil, nil) if EOF is reached without a match.
				if err == io.EOF {
					err = nil
				}
				return err
			}
		}
	}
}

func parsePasswd(fs fs.FS, path string) (map[int]string, error) {
	users := make(map[int]string)
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}

	if err := readColonFile(file, func(line []byte) any {
		if bytes.Count(line, colon) < 6 {
			return nil
		}
		// kevin:x:1005:1006::/home/kevin:/usr/bin/zsh
		parts := strings.SplitN(string(line), ":", 7)
		if len(parts) < 6 || parts[0] == "" ||
			parts[0][0] == '+' || parts[0][0] == '-' {
			return nil
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil
		}
		users[uid] = parts[0]
		return uid
	}, 6); err != nil {
		return nil, err
	}

	return users, nil
}

func parseGroup(fs fs.FS, path string) (map[int]string, error) {
	groups := make(map[int]string)
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}

	if err := readColonFile(file, func(line []byte) any {
		if bytes.Count(line, colon) < 3 {
			return nil
		}
		// wheel:*:0:root
		parts := strings.SplitN(string(line), ":", 4)
		if len(parts) < 4 || parts[0] == "" ||
			// If the file contains +foo and you search for "foo", glibc
			// returns an "invalid argument" error. Similarly, if you search
			// for a gid for a row where the group name starts with "+" or "-",
			// glibc fails to find the record.
			parts[0][0] == '+' || parts[0][0] == '-' {
			return nil
		}
		gid, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil
		}
		groups[gid] = parts[0]
		return gid
	}, 6); err != nil {
		return nil, err
	}

	return groups, nil
}
