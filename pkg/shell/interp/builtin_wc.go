// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"bufio"
	"context"
	"io"
	"os"
	"unicode/utf8"
)

// builtinWc implements the POSIX wc command.
// Options: -l (lines), -w (words), -c (bytes), -m (chars).
// Default (no flags): lines + words + bytes.
func (r *Runner) builtinWc(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		showLines bool
		showWords bool
		showBytes bool
		showChars bool
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-l":
			showLines = true
		case "-w":
			showWords = true
		case "-c":
			showBytes = true
		case "-m":
			showChars = true
		default:
			r.errf("wc: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	// Default: show all three classic counts.
	if !showLines && !showWords && !showBytes && !showChars {
		showLines = true
		showWords = true
		showBytes = true
	}

	paths := fp.args()

	var totalLines, totalWords, totalBytes, totalChars int64

	printCounts := func(lines, words, bytes, chars int64, name string) {
		if showLines {
			r.outf("%8d", lines)
		}
		if showWords {
			r.outf("%8d", words)
		}
		if showBytes {
			r.outf("%8d", bytes)
		}
		if showChars {
			r.outf("%8d", chars)
		}
		if name != "" {
			r.outf(" %s", name)
		}
		r.out("\n")
	}

	if len(paths) == 0 {
		if r.stdin == nil {
			r.errf("wc: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		lines, words, bytes, chars := wcCount(r.stdin)
		printCounts(lines, words, bytes, chars, "")
		return exit
	}

	for _, p := range paths {
		var reader io.Reader
		var closer io.Closer

		if p == "-" {
			if r.stdin == nil {
				continue
			}
			reader = r.stdin
		} else {
			absP := r.absPath(p)
			f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
			if err != nil {
				r.errf("wc: %v\n", err)
				exit.code = 1
				continue
			}
			reader = f
			closer = f
		}

		lines, words, bytes, chars := wcCount(reader)
		if closer != nil {
			closer.Close()
		}

		totalLines += lines
		totalWords += words
		totalBytes += bytes
		totalChars += chars

		printCounts(lines, words, bytes, chars, p)
	}

	if len(paths) > 1 {
		printCounts(totalLines, totalWords, totalBytes, totalChars, "total")
	}

	return exit
}

// wcCount counts lines, words, bytes, and characters in a reader.
func wcCount(reader io.Reader) (lines, words, bytes, chars int64) {
	br := bufio.NewReader(reader)
	inWord := false

	for {
		buf := make([]byte, 4096)
		n, err := br.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			bytes += int64(n)
			chars += int64(utf8.RuneCount(chunk))

			for _, b := range chunk {
				switch b {
				case '\n':
					lines++
					if inWord {
						words++
						inWord = false
					}
				case ' ', '\t', '\r', '\v', '\f':
					if inWord {
						words++
						inWord = false
					}
				default:
					inWord = true
				}
			}
		}
		if err != nil {
			break
		}
	}

	if inWord {
		words++
	}

	return
}
