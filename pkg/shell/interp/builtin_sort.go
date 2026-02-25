// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"bufio"
	"cmp"
	"context"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// builtinSort implements the POSIX sort command.
// Options: -r, -n, -f, -u, -b, -t, -k, -s.
// Safety: -o (output file) is explicitly rejected.
func (r *Runner) builtinSort(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		reverse     bool
		numeric     bool
		foldCase    bool
		unique      bool
		ignoreBlanks bool
		separator   string
		keyDefs     []sortKeyDef
		stable      bool
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-r":
			reverse = true
		case "-n":
			numeric = true
		case "-f":
			foldCase = true
		case "-u":
			unique = true
		case "-b":
			ignoreBlanks = true
		case "-s":
			stable = true
		case "-t":
			v := fp.value()
			if v == "" {
				r.errf("sort: option requires an argument -- 't'\n")
				exit.code = 2
				return exit
			}
			separator = v
		case "-k":
			v := fp.value()
			if v == "" {
				r.errf("sort: option requires an argument -- 'k'\n")
				exit.code = 2
				return exit
			}
			kd, err := parseSortKey(v)
			if err != nil {
				r.errf("sort: invalid key: %q\n", v)
				exit.code = 2
				return exit
			}
			keyDefs = append(keyDefs, kd)
		case "-o":
			r.errf("sort: -o (output file) is not available in safe shell\n")
			exit.code = 2
			return exit
		default:
			r.errf("sort: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	paths := fp.args()

	// Collect all lines.
	var lines []string

	addLines := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
	}

	if len(paths) == 0 {
		if r.stdin == nil {
			r.errf("sort: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		addLines(r.stdin)
	} else {
		for _, p := range paths {
			absP := r.absPath(p)
			f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
			if err != nil {
				r.errf("sort: %v\n", err)
				exit.code = 2
				return exit
			}
			addLines(f)
			f.Close()
		}
	}

	// Build comparison function.
	cmpFn := func(a, b string) int {
		ka := sortExtractKey(a, keyDefs, separator, ignoreBlanks)
		kb := sortExtractKey(b, keyDefs, separator, ignoreBlanks)

		if numeric {
			na := sortParseNum(ka)
			nb := sortParseNum(kb)
			c := cmp.Compare(na, nb)
			if c != 0 {
				if reverse {
					return -c
				}
				return c
			}
		}

		ca, cb := ka, kb
		if foldCase {
			ca = strings.ToLower(ca)
			cb = strings.ToLower(cb)
		}
		c := cmp.Compare(ca, cb)
		if reverse {
			c = -c
		}
		return c
	}

	if stable {
		slices.SortStableFunc(lines, cmpFn)
	} else {
		slices.SortFunc(lines, cmpFn)
	}

	// Output, with optional deduplication.
	var prev *string
	for _, line := range lines {
		if unique && prev != nil && cmpFn(*prev, line) == 0 {
			continue
		}
		r.outf("%s\n", line)
		prev = &line
	}

	return exit
}

// sortKeyDef represents a -k field specification.
type sortKeyDef struct {
	startField int // 1-based
	startChar  int // 1-based, 0 = start of field
	endField   int // 1-based, 0 = end of line
	endChar    int // 1-based, 0 = end of field
}

// parseSortKey parses a -k KEYDEF like "1", "1,2", "1.2,3.4".
func parseSortKey(s string) (sortKeyDef, error) {
	var kd sortKeyDef
	parts := strings.SplitN(s, ",", 2)

	start := parts[0]
	if dotIdx := strings.IndexByte(start, '.'); dotIdx >= 0 {
		n, err := strconv.Atoi(start[:dotIdx])
		if err != nil || n < 1 {
			return kd, err
		}
		kd.startField = n
		n, err = strconv.Atoi(start[dotIdx+1:])
		if err != nil || n < 1 {
			return kd, err
		}
		kd.startChar = n
	} else {
		n, err := strconv.Atoi(start)
		if err != nil || n < 1 {
			return kd, err
		}
		kd.startField = n
	}

	if len(parts) == 2 {
		end := parts[1]
		if dotIdx := strings.IndexByte(end, '.'); dotIdx >= 0 {
			n, err := strconv.Atoi(end[:dotIdx])
			if err != nil || n < 1 {
				return kd, err
			}
			kd.endField = n
			n, err = strconv.Atoi(end[dotIdx+1:])
			if err != nil || n < 1 {
				return kd, err
			}
			kd.endChar = n
		} else {
			n, err := strconv.Atoi(end)
			if err != nil || n < 1 {
				return kd, err
			}
			kd.endField = n
		}
	}

	return kd, nil
}

// sortExtractKey extracts the sort key from a line using keyDefs and separator.
func sortExtractKey(line string, keyDefs []sortKeyDef, separator string, ignoreBlanks bool) string {
	if len(keyDefs) == 0 {
		if ignoreBlanks {
			return strings.TrimLeft(line, " \t")
		}
		return line
	}

	kd := keyDefs[0] // Use first key definition for comparison.
	fields := sortSplitFields(line, separator)

	startIdx := kd.startField - 1
	if startIdx >= len(fields) {
		return ""
	}

	endIdx := len(fields) - 1
	if kd.endField > 0 {
		endIdx = kd.endField - 1
		if endIdx >= len(fields) {
			endIdx = len(fields) - 1
		}
	}

	if startIdx > endIdx {
		return ""
	}

	result := strings.Join(fields[startIdx:endIdx+1], func() string {
		if separator != "" {
			return separator
		}
		return " "
	}())

	if ignoreBlanks {
		result = strings.TrimLeft(result, " \t")
	}

	return result
}

// sortSplitFields splits a line into fields by separator.
func sortSplitFields(line, separator string) []string {
	if separator != "" {
		return strings.Split(line, separator)
	}
	return strings.Fields(line)
}

// sortParseNum extracts a leading numeric value from a string.
func sortParseNum(s string) float64 {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return 0
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	var n float64
	var decimal float64
	inDecimal := false

	for _, c := range s {
		if c >= '0' && c <= '9' {
			if inDecimal {
				decimal /= 10
				n += float64(c-'0') * decimal
			} else {
				n = n*10 + float64(c-'0')
			}
		} else if c == '.' && !inDecimal {
			inDecimal = true
			decimal = 1
		} else {
			break
		}
	}

	if negative {
		n = -n
	}
	return n
}
