// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package strings_cmd implements the strings builtin command.
//
// strings — print the sequences of printable characters in files
//
// Usage: strings [OPTION]... [FILE]...
//
// Print printable character sequences in files.
// With no FILE, or when FILE is -, read standard input.
//
// A printable character is any byte in the range 0x20–0x7e (inclusive), or
// a horizontal tab (0x09). Sequences shorter than the minimum length (default
// 4) are silently discarded.
//
// Accepted flags:
//
//	-a, --all
//	    Scan the entire file (already the default; accepted for POSIX
//	    compatibility).
//
//	-n min-len, --bytes=min-len
//	    Minimum sequence length to report (default 4). Must be >= 1.
//
//	-t format, --radix=format
//	    Print the file byte offset before each string, formatted according
//	    to format:
//	      o = octal
//	      d = decimal
//	      x = hexadecimal
//	    The offset is right-justified in a 7-character field followed by a
//	    single space, matching GNU strings output.
//
//	-o
//	    Legacy alias for -t o (octal offsets).
//
//	-f, --print-file-name
//	    Print the file name before each string.
//
//	-s separator, --output-separator=separator
//	    Use separator instead of a newline after each string.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one error occurred (missing file, permission denied, etc.).
//
// Memory safety:
//
//	Input is read in 32 KiB chunks. Individual strings are capped at
//	maxStringLen (1 MiB) to prevent memory exhaustion — the first 1 MiB of
//	an extremely long printable run is emitted and scanning continues.
//	All read loops check ctx.Err() at each iteration to honour the shell's
//	execution timeout and support graceful cancellation.
package strings_cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the strings builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "strings",
	Description: "print printable character sequences",
	MakeFlags:   registerFlags,
}

const (
	defaultMinLen = 4
	maxStringLen  = 1 << 20 // 1 MiB per string, to prevent memory exhaustion
	readChunkSize = 32 * 1024
	maxMinLen     = 1<<31 - 1 // math.MaxInt32 without importing math
)

type radixFormat byte

const (
	radixNone    radixFormat = 0
	radixOctal   radixFormat = 'o'
	radixDecimal radixFormat = 'd'
	radixHex     radixFormat = 'x'
)

// radixFlagVal implements pflag.Value for the -t / --radix flag.
// Validation happens in Set so pflag reports errors during parsing, which also
// correctly rejects empty values (e.g. --radix= or -t ”).
type radixFlagVal struct{ target *radixFormat }

func (r *radixFlagVal) String() string {
	switch *r.target {
	case radixOctal:
		return "o"
	case radixDecimal:
		return "d"
	case radixHex:
		return "x"
	default:
		return ""
	}
}

func (r *radixFlagVal) Set(s string) error {
	switch s {
	case "o":
		*r.target = radixOctal
	case "d":
		*r.target = radixDecimal
	case "x":
		*r.target = radixHex
	default:
		return errors.New("invalid radix")
	}
	return nil
}

func (r *radixFlagVal) Type() string { return "string" }

// octalFlagVal implements pflag.Value for the legacy -o flag (alias for -t o).
// Both -o and -t share the same *radixFormat target so pflag's left-to-right
// Set() calls naturally implement last-flag-wins semantics.
type octalFlagVal struct{ target *radixFormat }

func (o *octalFlagVal) String() string { return "false" }

func (o *octalFlagVal) Set(s string) error {
	if s == "true" {
		*o.target = radixOctal
	}
	return nil
}

func (o *octalFlagVal) IsBoolFlag() bool { return true }
func (o *octalFlagVal) Type() string     { return "bool" }

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	_ = fs.BoolP("all", "a", false, "scan entire file (default; accepted for POSIX compatibility)")
	minLen := fs.IntP("bytes", "n", defaultMinLen, "minimum string length (default 4)")
	// format is shared by both -t and -o; pflag calls Set() in parse order so
	// whichever flag appears last on the command line wins (last-flag-wins).
	var format radixFormat
	fs.VarP(&radixFlagVal{target: &format}, "radix", "t", "print file offset in given radix: o=octal, d=decimal, x=hex")
	// NoOptDefVal = "true" makes pflag treat -o as a no-argument boolean flag
	// (same as BoolVarP does internally), so -o alone calls Set("true").
	oFlag := fs.VarPF(&octalFlagVal{target: &format}, "offset-octal", "o", "alias for -t o (print octal offsets)")
	oFlag.NoOptDefVal = "true"
	printFileName := fs.BoolP("print-file-name", "f", false, "print file name before each string")
	separator := fs.StringP("output-separator", "s", "\n", "output separator between strings (default newline)")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		// Validate -n / --bytes BEFORE the --help short-circuit so
		// that `strings -n 0 --help` exits with "invalid minimum
		// string length" instead of printing help. GNU strings runs
		// the bounds check during option processing; the shared
		// args-trim in builtins/builtins.go relies on this ordering to
		// safely honour `--help` after value-taking flags. The default
		// (defaultMinLen=4) is in-range, so `strings --help` (no -n)
		// still short-circuits cleanly.
		if *minLen < 1 || *minLen > maxMinLen {
			callCtx.Errf("strings: invalid minimum string length %d\n", *minLen)
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: strings [OPTION]... [FILE]...\n")
			callCtx.Out("Print printable character sequences in files.\n")
			callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		// format is already resolved: pflag called Set() on the custom flag values
		// in parse order, so the last of -o / -t wins (same as GNU strings).

		files := args
		if len(files) == 0 {
			files = []string{"-"}
		}

		opts := options{
			minLen:    *minLen,
			format:    format,
			printFile: *printFileName,
			sep:       *separator,
		}

		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				break
			}
			if err := processFile(ctx, callCtx, file, opts); err != nil {
				name := file
				if file == "-" {
					name = "standard input"
				}
				callCtx.Errf("%s", fmtFileError(name, err))
				failed = true
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

type options struct {
	minLen    int
	format    radixFormat
	printFile bool
	sep       string
}

func processFile(ctx context.Context, callCtx *builtins.CallContext, file string, opts options) error {
	// Check for directories before opening, matching GNU strings behavior.
	if file != "-" {
		if info, err := callCtx.StatFile(ctx, file); err == nil && info.IsDir() {
			return errIsDirectory
		}
	}
	rc, err := openReader(ctx, callCtx, file)
	if err != nil {
		return err
	}
	if rc == nil {
		return nil
	}
	defer rc.Close()

	buf := make([]byte, readChunkSize)
	var current []byte    // current printable sequence being accumulated
	var stringStart int64 // file offset where the current sequence started
	var fileOffset int64  // running byte offset in the file

	flush := func() {
		if len(current) >= opts.minLen {
			if opts.printFile {
				callCtx.Outf("%s: ", file)
			}
			if opts.format != radixNone {
				callCtx.Stdout.Write(fmtOffset(stringStart, opts.format)) //nolint:errcheck
			}
			callCtx.Stdout.Write(current) //nolint:errcheck
			callCtx.Out(opts.sep)
		}
		current = current[:0]
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, readErr := rc.Read(buf)
		for i := 0; i < n; i++ {
			b := buf[i]
			if isPrintable(b) {
				if len(current) == 0 {
					stringStart = fileOffset + int64(i)
				}
				if len(current) < maxStringLen {
					current = append(current, b)
				}
			} else {
				flush()
			}
		}
		fileOffset += int64(n)

		if errors.Is(readErr, io.EOF) {
			flush()
			return nil
		}
		if readErr != nil {
			flush()
			return readErr
		}
	}
}

// isPrintable returns true for bytes 0x20–0x7e (printable ASCII) and 0x09 (tab).
func isPrintable(b byte) bool {
	return (b >= 0x20 && b <= 0x7e) || b == '\t'
}

// fmtOffset formats offset right-justified in a 7-character field followed by a space,
// matching the GNU strings output format.
func fmtOffset(offset int64, format radixFormat) []byte {
	var base int
	switch format {
	case radixOctal:
		base = 8
	case radixDecimal:
		base = 10
	case radixHex:
		base = 16
	default:
		// unreachable: format is validated in registerFlags
		base = 16
	}
	s := strconv.FormatInt(offset, base)
	const width = 7
	out := make([]byte, 0, width+1)
	for i := len(s); i < width; i++ {
		out = append(out, ' ')
	}
	out = append(out, s...)
	out = append(out, ' ')
	return out
}

// errIsDirectory is a sentinel indicating the path is a directory.
var errIsDirectory = errors.New("is a directory")

// fmtFileError formats a file error message matching GNU strings output.
// GNU strings uses:
//
//	strings: 'file': No such file         (for missing files)
//	strings: Warning: 'dir' is a directory (for directories)
func fmtFileError(name string, err error) string {
	if errors.Is(err, errIsDirectory) {
		return fmt.Sprintf("strings: Warning: '%s' is a directory\n", name)
	}
	// The sandbox normalizes OS errors via portablePathError, so check
	// the inner error string for portable "no such file or directory".
	msg := innerErrMsg(err)
	if msg == "no such file or directory" {
		return fmt.Sprintf("strings: '%s': No such file\n", name)
	}
	return fmt.Sprintf("strings: '%s': %s\n", name, msg)
}

// innerErrMsg extracts the innermost error message, unwrapping PathError.
func innerErrMsg(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func openReader(ctx context.Context, callCtx *builtins.CallContext, file string) (io.ReadCloser, error) {
	if file == "-" {
		if callCtx.Stdin == nil {
			return nil, nil
		}
		return io.NopCloser(callCtx.Stdin), nil
	}
	return callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
}
