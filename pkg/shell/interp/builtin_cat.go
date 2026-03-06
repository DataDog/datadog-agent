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
)

const catLargeFileWarningBytes = 512 * 1024 // 512 KiB — warn before the 1MB output cap hits

// builtinCat implements the POSIX cat command.
// Options: -n (number all lines), -b (number non-blank, overrides -n), -s (squeeze blank lines).
// Safety: Files > 512KB trigger a warning to stderr suggesting head/tail/grep.
func (r *Runner) builtinCat(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		numberAll   bool // -n
		numberBlank bool // -b (overrides -n)
		squeeze     bool // -s
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-n":
			numberAll = true
		case "-b":
			numberBlank = true
		case "-s":
			squeeze = true
		default:
			r.errf("cat: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	// -b overrides -n
	if numberBlank {
		numberAll = false
	}

	paths := fp.args()

	lineNum := 1

	if len(paths) == 0 {
		// Read from stdin.
		if r.stdin == nil {
			r.errf("cat: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		catStream(r, r.stdin, numberAll, numberBlank, squeeze, &lineNum)
		return exit
	}

	for _, p := range paths {
		if p == "-" {
			if r.stdin == nil {
				continue
			}
			catStream(r, r.stdin, numberAll, numberBlank, squeeze, &lineNum)
			continue
		}

		absP := r.absPath(p)

		// Safety: warn on large files before reading.
		info, err := r.stat(ctx, absP)
		if err == nil && info.Size() > catLargeFileWarningBytes {
			sizeMB := float64(info.Size()) / (1024 * 1024)
			r.errf("cat: %s is %.1fMB, output may be truncated (1MB cap). Consider using head, tail, or grep for large files.\n", p, sizeMB)
		}

		f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
		if err != nil {
			r.errf("cat: %v\n", err)
			exit.code = 1
			continue
		}

		catStream(r, f, numberAll, numberBlank, squeeze, &lineNum)
		f.Close()
	}

	return exit
}

// catStream reads from a reader and outputs with optional line numbering and blank squeezing.
func catStream(r *Runner, reader io.Reader, numberAll, numberBlank, squeeze bool, lineNum *int) {
	scanner := bufio.NewScanner(reader)
	prevBlank := false

	for scanner.Scan() {
		line := scanner.Text()
		isBlank := len(line) == 0

		if squeeze && isBlank && prevBlank {
			continue
		}
		prevBlank = isBlank

		if numberBlank {
			if isBlank {
				r.out("\n")
			} else {
				r.outf("%6d\t%s\n", *lineNum, line)
				*lineNum++
			}
		} else if numberAll {
			r.outf("%6d\t%s\n", *lineNum, line)
			*lineNum++
		} else {
			r.outf("%s\n", line)
		}
	}
	if err := scanner.Err(); err != nil {
		r.errf("cat: read error: %v\n", err)
	}
}
