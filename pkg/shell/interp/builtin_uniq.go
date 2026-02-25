// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
)

// builtinUniq implements the POSIX uniq command.
// Options: -c, -d, -u, -i, -f N, -s N, -w N.
// Safety: Output file (second positional arg) is rejected.
func (r *Runner) builtinUniq(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		showCount      bool // -c
		dupsOnly       bool // -d
		uniqOnly       bool // -u
		ignoreCase     bool // -i
		skipFields     int  // -f N
		skipChars      int  // -s N
		compareWidth   int  // -w N (0 = full width)
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-c":
			showCount = true
		case "-d":
			dupsOnly = true
		case "-u":
			uniqOnly = true
		case "-i":
			ignoreCase = true
		case "-f":
			v := fp.value()
			if v == "" {
				r.errf("uniq: option requires an argument -- 'f'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				r.errf("uniq: invalid number of fields to skip: %q\n", v)
				exit.code = 2
				return exit
			}
			skipFields = n
		case "-s":
			v := fp.value()
			if v == "" {
				r.errf("uniq: option requires an argument -- 's'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				r.errf("uniq: invalid number of chars to skip: %q\n", v)
				exit.code = 2
				return exit
			}
			skipChars = n
		case "-w":
			v := fp.value()
			if v == "" {
				r.errf("uniq: option requires an argument -- 'w'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				r.errf("uniq: invalid number of chars to compare: %q\n", v)
				exit.code = 2
				return exit
			}
			compareWidth = n
		default:
			r.errf("uniq: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	positional := fp.args()
	if len(positional) > 1 {
		r.errf("uniq: output file is not available in safe shell\n")
		exit.code = 2
		return exit
	}

	var reader io.Reader
	var closer io.Closer

	if len(positional) == 0 {
		if r.stdin == nil {
			r.errf("uniq: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		reader = r.stdin
	} else {
		absP := r.absPath(positional[0])
		f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
		if err != nil {
			r.errf("uniq: %v\n", err)
			exit.code = 1
			return exit
		}
		reader = f
		closer = f
	}

	scanner := bufio.NewScanner(reader)

	extractKey := func(line string) string {
		key := line

		// Skip fields.
		if skipFields > 0 {
			remaining := key
			for i := 0; i < skipFields; i++ {
				remaining = strings.TrimLeft(remaining, " \t")
				idx := strings.IndexAny(remaining, " \t")
				if idx < 0 {
					remaining = ""
					break
				}
				remaining = remaining[idx:]
			}
			key = strings.TrimLeft(remaining, " \t")
		}

		// Skip chars.
		if skipChars > 0 {
			runes := []rune(key)
			if skipChars < len(runes) {
				key = string(runes[skipChars:])
			} else {
				key = ""
			}
		}

		// Compare width.
		if compareWidth > 0 {
			runes := []rune(key)
			if compareWidth < len(runes) {
				key = string(runes[:compareWidth])
			}
		}

		if ignoreCase {
			key = strings.ToLower(key)
		}

		return key
	}

	outputLine := func(line string, count int) {
		if dupsOnly && count < 2 {
			return
		}
		if uniqOnly && count > 1 {
			return
		}
		if showCount {
			r.outf("%7d %s\n", count, line)
		} else {
			r.outf("%s\n", line)
		}
	}

	var prevLine string
	var prevKey string
	count := 0
	first := true

	for scanner.Scan() {
		line := scanner.Text()
		key := extractKey(line)

		if first {
			prevLine = line
			prevKey = key
			count = 1
			first = false
			continue
		}

		if key == prevKey {
			count++
		} else {
			outputLine(prevLine, count)
			prevLine = line
			prevKey = key
			count = 1
		}
	}

	if !first {
		outputLine(prevLine, count)
	}

	if closer != nil {
		closer.Close()
	}

	return exit
}
