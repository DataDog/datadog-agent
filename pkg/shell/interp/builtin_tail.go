// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// builtinTail implements the POSIX tail command.
// Supported flags: -f, -c number, -n number.
// The number may be prefixed with + to count from the beginning of the file.
func (r *Runner) builtinTail(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		follow   bool
		lineMode = true // -n (default) vs -c
		countStr = "10" // default: last 10 lines
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-f":
			follow = true
		case "-n":
			lineMode = true
			countStr = fp.value()
			if countStr == "" {
				r.errf("tail: option requires an argument -- 'n'\n")
				exit.code = 2
				return exit
			}
		case "-c":
			lineMode = false
			countStr = fp.value()
			if countStr == "" {
				r.errf("tail: option requires an argument -- 'c'\n")
				exit.code = 2
				return exit
			}
		default:
			r.errf("tail: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	// Parse the count value. A leading '+' means from the beginning.
	fromBeginning := false
	s := countStr
	if strings.HasPrefix(s, "+") {
		fromBeginning = true
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	count, err := strconv.ParseInt(s, 10, 64)
	if err != nil || count < 0 {
		if lineMode {
			r.errf("tail: invalid number of lines: %q\n", countStr)
		} else {
			r.errf("tail: invalid number of bytes: %q\n", countStr)
		}
		exit.code = 2
		return exit
	}

	paths := fp.args()

	// No file arguments: read from stdin.
	if len(paths) == 0 {
		if r.stdin == nil {
			r.errf("tail: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		data, _ := io.ReadAll(r.stdin)
		if lineMode {
			tailLastLines(r, data, count, fromBeginning)
		} else {
			tailLastBytes(r, data, count, fromBeginning)
		}
		return exit
	}

	multipleFiles := len(paths) > 1

	for i, p := range paths {
		absP := r.absPath(p)

		if multipleFiles {
			if i > 0 {
				r.out("\n")
			}
			r.outf("==> %s <==\n", p)
		}

		f, err := r.open(ctx, absP, os.O_RDONLY, 0, false)
		if err != nil {
			r.errf("tail: cannot open '%s' for reading: %v\n", p, err)
			exit.code = 1
			continue
		}

		data, _ := io.ReadAll(f)
		if lineMode {
			tailLastLines(r, data, count, fromBeginning)
		} else {
			tailLastBytes(r, data, count, fromBeginning)
		}

		// Follow mode: only the last file is followed (POSIX behaviour).
		if follow && i == len(paths)-1 {
			tailFollow(ctx, r, f)
		} else {
			f.Close()
		}
	}

	return exit
}

// tailSplitLines splits data into lines, each retaining its trailing '\n'
// (except possibly the last line if the data does not end with '\n').
func tailSplitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var lines []string
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			lines = append(lines, string(data))
			break
		}
		lines = append(lines, string(data[:idx+1]))
		data = data[idx+1:]
	}
	return lines
}

// tailLastLines outputs lines from data.
//
// When fromBeginning is true, count is a 1-based line number: output starts
// from that line onward (e.g. +1 = entire file, +2 = skip first line).
//
// Otherwise, the last count lines are output.
func tailLastLines(r *Runner, data []byte, count int64, fromBeginning bool) {
	lines := tailSplitLines(data)
	if len(lines) == 0 {
		return
	}

	if fromBeginning {
		start := count - 1 // 1-based to 0-based
		if start < 0 {
			start = 0
		}
		for i := start; i < int64(len(lines)); i++ {
			r.out(lines[i])
		}
		return
	}

	if count == 0 {
		return
	}

	start := int64(len(lines)) - count
	if start < 0 {
		start = 0
	}
	for i := start; i < int64(len(lines)); i++ {
		r.out(lines[i])
	}
}

// tailLastBytes outputs bytes from data.
//
// When fromBeginning is true, count is a 1-based byte offset.
// Otherwise, the last count bytes are output.
func tailLastBytes(r *Runner, data []byte, count int64, fromBeginning bool) {
	if len(data) == 0 {
		return
	}

	if fromBeginning {
		offset := count - 1 // 1-based to 0-based
		if offset < 0 {
			offset = 0
		}
		if offset < int64(len(data)) {
			r.out(string(data[offset:]))
		}
		return
	}

	if count == 0 {
		return
	}

	if int64(len(data)) <= count {
		r.out(string(data))
	} else {
		r.out(string(data[int64(len(data))-count:]))
	}
}

const tailFollowMaxDuration = 60 * time.Second

// tailFollow continuously reads and outputs new data appended to a file.
// It polls once per second and stops when the context is cancelled or
// the maximum follow duration (60s) is reached.
func tailFollow(ctx context.Context, r *Runner, reader io.ReadCloser) {
	ctx, cancel := context.WithTimeout(ctx, tailFollowMaxDuration)
	defer cancel()
	defer reader.Close()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			r.out(string(buf[:n]))
			continue // try reading more immediately
		}
		if err != nil && err != io.EOF {
			return // real error, stop following
		}
		// At EOF, sleep and try again.
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
