// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
)

// builtinHead implements the POSIX head command.
// Options: -n COUNT (lines, default 10), -c COUNT (bytes), -q (quiet), -v (verbose headers).
func (r *Runner) builtinHead(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		lineMode = true
		count    = int64(10)
		quiet    bool
		verbose  bool
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-n":
			lineMode = true
			v := fp.value()
			if v == "" {
				r.errf("head: option requires an argument -- 'n'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				r.errf("head: invalid number of lines: %q\n", v)
				exit.code = 2
				return exit
			}
			count = n
		case "-c":
			lineMode = false
			v := fp.value()
			if v == "" {
				r.errf("head: option requires an argument -- 'c'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				r.errf("head: invalid number of bytes: %q\n", v)
				exit.code = 2
				return exit
			}
			count = n
		case "-q":
			quiet = true
			verbose = false
		case "-v":
			verbose = true
			quiet = false
		default:
			r.errf("head: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	paths := fp.args()

	if len(paths) == 0 {
		if r.stdin == nil {
			r.errf("head: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		if lineMode {
			headLines(r, r.stdin, count)
		} else {
			headBytes(r, r.stdin, count)
		}
		return exit
	}

	multipleFiles := len(paths) > 1

	for i, p := range paths {
		if multipleFiles && !quiet || verbose {
			if i > 0 {
				r.out("\n")
			}
			r.outf("==> %s <==\n", p)
		}

		absP := r.absPath(p)
		f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
		if err != nil {
			r.errf("head: cannot open '%s' for reading: %v\n", p, err)
			exit.code = 1
			continue
		}

		if lineMode {
			headLines(r, f, count)
		} else {
			headBytes(r, f, count)
		}
		f.Close()
	}

	return exit
}

func headLines(r *Runner, reader io.Reader, n int64) {
	scanner := bufio.NewScanner(reader)
	var printed int64
	for scanner.Scan() {
		if printed >= n {
			break
		}
		r.outf("%s\n", scanner.Text())
		printed++
	}
	if err := scanner.Err(); err != nil {
		r.errf("head: read error: %v\n", err)
	}
}

func headBytes(r *Runner, reader io.Reader, n int64) {
	buf := make([]byte, 4096)
	var written int64
	for written < n {
		toRead := int64(len(buf))
		if remaining := n - written; remaining < toRead {
			toRead = remaining
		}
		nr, err := reader.Read(buf[:toRead])
		if nr > 0 {
			r.out(string(buf[:nr]))
			written += int64(nr)
		}
		if err != nil {
			break
		}
	}
}
