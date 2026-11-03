// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package head implements the head builtin command.
//
// head — output the first part of files
//
// Usage: head [OPTION]... [FILE]...
//
// Print the first 10 lines of each FILE to standard output.
// With more than one FILE, precede each with a header giving the file name.
// With no FILE, or when FILE is -, read standard input.
//
// Accepted flags:
//
//	-n N, --lines=N
//	    Output the first N lines (default 10). A leading '+' (e.g. +5) is
//	    treated as a positive sign and is equivalent to plain 5.
//
//	-c N, --bytes=N
//	    Output the first N bytes instead of lines. A leading '+' is treated
//	    as a positive sign. If both -n and -c are specified, the last flag
//	    on the command line takes effect.
//
//	-q, --quiet, --silent
//	    Never print file name headers. --silent is an alias for --quiet.
//
//	-v, --verbose
//	    Always print file name headers, even when only one file is given.
//
//	-h, --help
//	    Print this usage message to stdout and exit 0.
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one error occurred (missing file, invalid argument, etc.).
//
// Memory safety:
//
//	Line mode uses a streaming scanner with a per-line cap of MaxLineBytes
//	(1 MiB). Lines that exceed this cap cause an error rather than an
//	unbounded allocation. Byte mode reads in fixed-size chunks; it never
//	allocates proportionally to user-supplied N. All loops check ctx.Err()
//	at each iteration to honour the shell's execution timeout and to support
//	graceful cancellation.
package head

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the head builtin command descriptor.
var Cmd = builtins.Command{
	Name:          "head",
	Description:   "output the first part of files",
	MakeFlags:     registerFlags,
	NormalizeArgs: normalizeArgs,
}

// normalizeArgs rewrites legacy -N shorthand (e.g. -5) to -n N so that
// pflag can parse it. Only a bare -<digits> token in the first argument
// position is rewritten, matching GNU head behavior where the obsolete form
// is only accepted as the first option.
func normalizeArgs(args []string) []string {
	return builtins.NormalizeBareNumberArg(args, []string{"-n", "-c", "--lines", "--bytes"})
}

// MaxCount is the maximum accepted line or byte count. Values above this
// are clamped. This prevents huge theoretical allocations while remaining
// larger than any practical file.
const MaxCount = 1<<31 - 1 // 2 147 483 647

// MaxLineBytes is the per-line buffer cap for the line scanner. Lines
// longer than this are reported as an error instead of being buffered.
const MaxLineBytes = 1 << 20 // 1 MiB

// registerFlags registers all head flags on the framework-provided FlagSet and
// returns a bound handler whose flag variables are captured by closure. The
// framework calls Parse and passes positional arguments to the handler.
func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")

	// quietFlag, silentFlag, and verboseFlag share a sequence counter so that
	// after parsing we can determine which of -q/--quiet/--silent/-v/--verbose
	// appeared last on the command line — the last flag wins, matching GNU head.
	// NoOptDefVal is set to "true" so pflag treats these as boolean flags that
	// can be given without a "=value" argument (e.g. "--quiet" not "--quiet=true").
	var headerSeq int
	quietFlag := newBoolSeqFlag(&headerSeq)
	silentFlag := newBoolSeqFlag(&headerSeq)
	verboseFlag := newBoolSeqFlag(&headerSeq)
	fs.VarPF(quietFlag, "quiet", "q", "never print file name headers").NoOptDefVal = "true"
	fs.VarPF(silentFlag, "silent", "", "alias for --quiet").NoOptDefVal = "true"
	fs.VarPF(verboseFlag, "verbose", "v", "always print file name headers").NoOptDefVal = "true"

	// linesFlag and bytesFlag share a sequence counter so that after parsing
	// we can compare their pos fields to determine which appeared last on the
	// command line. pflag calls Set() in parse order, so the last flag Set
	// gets the highest pos value — no raw arg scanning required.
	var modeSeq int
	linesFlag := newModeFlag(&modeSeq, "10")
	bytesFlag := newModeFlag(&modeSeq, "")
	fs.VarP(linesFlag, "lines", "n", "print the first N lines instead of the first 10")
	fs.VarP(bytesFlag, "bytes", "c", "print the first N bytes instead of lines")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		// Validate all explicitly set mode flags upfront, BEFORE the
		// --help short-circuit. GNU head processes options left-to-right
		// and validates each as it goes, so `head -n xyz --help` exits
		// with "invalid number of lines" rather than printing help; the
		// shared args-trim in builtins/builtins.go relies on this
		// ordering to safely honor `--help` after value-taking flags.
		// GNU also rejects invalid values for flags that are later
		// overridden by another value (e.g. `head -n xyz -n 1`); the
		// modeFlag.Set captures the FIRST invalid value rather than
		// just whichever was set last, so this check reports the same
		// value GNU would.
		//
		// When BOTH -n and -c have invalid values, report whichever
		// appeared first in argv (smaller invalidPos), again matching
		// GNU's left-to-right rule. E.g. `head -c bad -n nope` reports
		// the byte error; `head -n nope -c bad` reports the line one.
		if reportLinesInvalidFirst(linesFlag, bytesFlag) {
			callCtx.Errf("head: invalid number of lines: '%s'\n", builtins.SafeOperand(linesFlag.invalid))
			return builtins.Result{Code: 1}
		}
		if bytesFlag.hasInvalid {
			callCtx.Errf("head: invalid number of bytes: '%s'\n", builtins.SafeOperand(bytesFlag.invalid))
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: head [OPTION]... [FILE]...\n")
			callCtx.Out("Print the first 10 lines of each FILE to standard output.\n")
			callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		// Bytes mode wins if -c/--bytes was parsed after -n/--lines. When neither
		// is set both pos fields are 0 (false → line mode). When only one is set
		// the other stays 0, so the comparison selects correctly.
		useBytesMode := bytesFlag.pos > linesFlag.pos

		// Parse the count for the chosen mode (handles the default "10" for
		// linesFlag when neither flag was explicitly given).
		countStr := linesFlag.val
		modeLabel := "lines"
		if useBytesMode {
			countStr = bytesFlag.val
			modeLabel = "bytes"
		}

		// Both explicit -n/-c values are already validated above. The
		// only remaining failure path is the implicit default "10" for
		// the linesFlag when neither was set, which always parses.
		count, ok := parseCount(countStr)
		if !ok {
			callCtx.Errf("head: invalid number of %s: '%s'\n", modeLabel, builtins.SafeOperand(countStr))
			return builtins.Result{Code: 1}
		}

		// Default to stdin when no file arguments were given.
		if len(files) == 0 {
			files = []string{"-"}
		}

		// Header printing: the last of -q/--quiet/--silent and -v/--verbose wins
		// (matching GNU head). --silent is an alias for --quiet and shares the
		// same sequence counter. If none are specified, print headers only when
		// multiple files are given.
		lastQuietPos := max(quietFlag.pos, silentFlag.pos)
		var printHeaders bool
		switch {
		case verboseFlag.pos > lastQuietPos:
			printHeaders = true // -v was specified last
		case lastQuietPos > verboseFlag.pos:
			printHeaders = false // -q or --silent was specified last
		default:
			printHeaders = len(files) > 1 // neither: default multi-file behaviour
		}

		var failed bool
		var printedHeader bool
		for _, file := range files {
			if ctx.Err() != nil {
				break
			}
			hp, err := processFile(ctx, callCtx, file, printedHeader, printHeaders, useBytesMode, count)
			if err != nil {
				name := file
				if file == "-" {
					name = "standard input"
				}
				callCtx.Errf("head: %s: %s\n", name, callCtx.PortableErr(err))
				failed = true
			}
			if hp {
				// A header was printed for this file; subsequent files should
				// emit a blank-line separator before their header. This is set
				// regardless of whether a read error occurred so that the
				// separator is not lost when a file opens successfully (header
				// printed) but reading later fails.
				printedHeader = true
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// processFile opens and processes one file (or stdin for "-").
// prevHeaderPrinted reports whether a header was already emitted for a previous
// file; when true, a blank-line separator is printed before this file's header.
// It returns (headerPrinted, err): headerPrinted is true whenever a header line
// was actually written to output, regardless of whether a read error follows.
func processFile(ctx context.Context, callCtx *builtins.CallContext, file string, prevHeaderPrinted bool, printHeaders, useBytesMode bool, count int64) (headerPrinted bool, err error) {
	name := file
	if file == "-" {
		name = "standard input"
		// Print the header before the nil-stdin guard so that -v always
		// emits a header for stdin even when no input stream is present.
		if printHeaders {
			if prevHeaderPrinted {
				callCtx.Out("\n")
			}
			callCtx.Outf("==> %s <==\n", name)
			headerPrinted = true
		}
		if callCtx.Stdin == nil {
			return headerPrinted, nil
		}
		// Pass callCtx.Stdin directly (not wrapped in NopCloser) so that
		// readLines can seek back buffered bytes when stdin is seekable
		// (e.g. redirected from a file). This allows a second '-' operand
		// to continue reading from the correct stream position, matching
		// GNU head behaviour.
		r := callCtx.Stdin
		if useBytesMode {
			return headerPrinted, readBytes(ctx, callCtx, r, count)
		}
		return headerPrinted, readLines(ctx, callCtx, r, count)
	}
	f, ferr := callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
	if ferr != nil {
		return false, ferr
	}
	defer f.Close()
	// Header is printed after a successful open so that a file that
	// cannot be opened produces no header (matches GNU head behaviour).
	if printHeaders {
		if prevHeaderPrinted {
			callCtx.Out("\n")
		}
		callCtx.Outf("==> %s <==\n", name)
		headerPrinted = true
	}
	if useBytesMode {
		return headerPrinted, readBytes(ctx, callCtx, f, count)
	}
	return headerPrinted, readLines(ctx, callCtx, f, count)
}

// readLines writes the first count lines of r to callCtx.Stdout, preserving
// line endings exactly (including a missing final newline).
//
// When r implements io.ReadSeeker (e.g. stdin redirected from a file), readLines
// seeks back any bytes the internal scanner read ahead but did not include in
// the N-line output. This allows a subsequent read on the same stream (e.g. a
// second '-' operand) to start from the correct position, matching GNU head.
func readLines(ctx context.Context, callCtx *builtins.CallContext, r io.Reader, count int64) error {
	// Wrap r in a byte counter so we can compute how many bytes the scanner
	// consumed from the underlying source — needed for the seek-back below.
	cr := &byteCountReader{r: r}
	sc := bufio.NewScanner(cr)
	buf := make([]byte, 4096)
	sc.Buffer(buf, MaxLineBytes)
	sc.Split(scanLinesPreservingNewline)

	var emitted int64
	var bytesEmitted int64
	for emitted < count && sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		token := sc.Bytes()
		if _, err := callCtx.Stdout.Write(token); err != nil {
			return err
		}
		emitted++
		bytesEmitted += int64(len(token))
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// If the underlying reader supports seeking, rewind any bytes the scanner
	// read ahead from the source but did not include in the N-line output.
	// excess = bytes pulled from r by the scanner − bytes returned as tokens.
	//
	// *os.File always satisfies io.ReadSeeker but Seek fails at runtime for
	// non-seekable fds (pipes, sockets). We probe with a no-op Seek(0, Current)
	// first; if it fails the stream is non-seekable and we skip the rewind
	// (pipe read-ahead is accepted as consumed, matching OS behaviour).
	if rs, ok := r.(io.ReadSeeker); ok {
		if excess := cr.total - bytesEmitted; excess > 0 {
			if _, err := rs.Seek(0, io.SeekCurrent); err == nil {
				if _, err := rs.Seek(-excess, io.SeekCurrent); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// byteCountReader is an io.Reader wrapper that counts the total bytes read
// from the underlying reader. Used by readLines to calculate scanner read-ahead
// so that seekable streams can be rewound to the correct position.
type byteCountReader struct {
	r     io.Reader
	total int64
}

func (c *byteCountReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.total += int64(n)
	return n, err
}

// readBytes writes the first count bytes of r to callCtx.Stdout. It reads
// in fixed-size chunks; the buffer is capped at chunkSize but shrunk to
// count when count is smaller, avoiding unnecessary allocation for small
// byte requests (e.g. head -c 5).
func readBytes(ctx context.Context, callCtx *builtins.CallContext, r io.Reader, count int64) error {
	if count == 0 {
		return nil
	}
	const chunkSize = 32 * 1024
	buf := make([]byte, min(int64(chunkSize), count))
	remaining := count
	for remaining > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		toRead := min(int64(chunkSize), remaining)
		n, err := r.Read(buf[:toRead])
		if n > 0 {
			remaining -= int64(n)
			if _, werr := callCtx.Stdout.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reportLinesInvalidFirst reports whether `head` should surface the
// linesFlag invalid-value error rather than bytesFlag's. True when only
// -n had an invalid value, or both did and -n came first in argv
// (smaller invalidPos). Matches GNU's leftmost-bad-option rule.
func reportLinesInvalidFirst(lines, bytes *modeFlag) bool {
	if !lines.hasInvalid {
		return false
	}
	if !bytes.hasInvalid {
		return true
	}
	return lines.invalidPos < bytes.invalidPos
}

// parseCount parses a line or byte count string. A leading '+' is
// accepted (treated as a positive sign by strconv.ParseInt, matching GNU
// head behavior). Returns (count, true) on success, (0, false) on failure.
func parseCount(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	if n > MaxCount {
		n = MaxCount
	}
	return n, true
}

// modeFlag is a pflag.Value implementation for -n/--lines and -c/--bytes.
// Two modeFlag values share a *seq counter; each call to Set increments
// the counter and records the new value in pos. After pflag.Parse, comparing
// pos fields reveals which flag appeared last on the command line — without
// scanning raw args or inspecting individual characters of flag tokens.
//
// Set always returns nil so pflag.Parse never fails on a bad numeric value;
// validation happens in the bound handler. To match GNU's left-to-right
// option processing (which reports the FIRST invalid value, not whichever
// happened to be set last), Set records the first invalid input it sees in
// invalid/hasInvalid plus the seq value at the time (invalidPos). The
// handler picks whichever mode flag has the smaller invalidPos so that
// `head -c bad -n nope` reports the byte error and `head -n nope -c bad`
// reports the line error.
type modeFlag struct {
	val        string
	seq        *int   // shared per-invocation counter; incremented on every Set call
	pos        int    // counter value when Set was last called; 0 means never set
	invalid    string // first value rejected by parseCount, if any
	hasInvalid bool   // true if Set ever saw an invalid value
	invalidPos int    // seq value when hasInvalid was first set; 0 means never set
}

func newModeFlag(seq *int, defaultVal string) *modeFlag {
	return &modeFlag{val: defaultVal, seq: seq}
}

func (f *modeFlag) String() string { return f.val }
func (f *modeFlag) Set(s string) error {
	f.val = s
	*f.seq++
	f.pos = *f.seq
	if !f.hasInvalid {
		if _, ok := parseCount(s); !ok {
			f.invalid = s
			f.hasInvalid = true
			f.invalidPos = *f.seq
		}
	}
	return nil
}
func (f *modeFlag) Type() string { return "string" }

// boolSeqFlag is a pflag.Value implementation for boolean flags that share a
// sequence counter with other flags. After pflag.Parse, comparing the pos
// fields of flags that share a counter reveals which was specified last.
// This is used to implement last-flag-wins semantics for -q/--quiet/--silent
// versus -v/--verbose.
type boolSeqFlag struct {
	seq *int
	pos int
}

func newBoolSeqFlag(seq *int) *boolSeqFlag {
	return &boolSeqFlag{seq: seq}
}

func (f *boolSeqFlag) String() string { return "false" }
func (f *boolSeqFlag) Set(s string) error {
	// GNU head rejects --quiet=<value> and --verbose=<value> with an error.
	// With NoOptDefVal = "true", pflag calls Set("true") for bare --quiet and
	// Set("<value>") when an explicit =<value> is given. We accept only "true"
	// (the NoOptDefVal) and reject any other value to match GNU head behaviour.
	if s != "true" {
		return errors.New("option doesn't allow an argument")
	}
	*f.seq++
	f.pos = *f.seq
	return nil
}
func (f *boolSeqFlag) Type() string { return "bool" }

// Note: pflag does NOT use an IsBoolFlag() method for flags registered via
// VarP/VarPF. The mechanism that makes these flags accept no value argument
// (e.g. "--quiet" rather than "--quiet=true") is NoOptDefVal = "true", set
// at registration time. IsBoolFlag() is intentionally absent here to avoid
// misleading future readers into thinking it is the active mechanism.

// scanLinesPreservingNewline is a bufio.SplitFunc that includes the line
// terminator (\n) in the returned token. Unlike bufio.ScanLines, it does not
// strip \r\n or \n, so the caller reproduces the exact file content. If the
// file's last line has no terminator, the bare bytes are returned as the
// final token.
func scanLinesPreservingNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i+1], nil
		}
	}
	if atEOF {
		// Last line has no trailing newline; return what we have.
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}
