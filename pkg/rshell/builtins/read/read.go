// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package read implements the read builtin command.
//
// read — read a line from standard input and assign to shell variables
//
// Usage: read [-r] [-p PROMPT] [-d DELIM] [-n NCHARS] [-N NBYTES] [-t TIMEOUT] [NAME...]
//
// Read one record from standard input (terminated by newline by default,
// or by DELIM with -d), apply POSIX field-splitting based on IFS, and
// assign each field to a shell variable named by NAME. With no NAME the
// whole line is assigned to REPLY. EOF returns exit code 1; a positive
// -t TIMEOUT that fires returns 142 with any partially-read data still
// assigned (matching bash's 128 + SIGALRM convention).
//
// Flags:
//
//	-r          Raw mode. Disable backslash-escape interpretation:
//	            backslashes are kept verbatim, no line continuation.
//	-p PROMPT   Print PROMPT to stderr before reading. Suppressed unless
//	            stdin is a terminal (matches bash).
//	-d DELIM    Use the first byte of DELIM as the line terminator
//	            instead of newline. An empty DELIM means NUL.
//	-n NCHARS   Return after reading at most NCHARS characters (max
//	            1 MiB). Counts UTF-8 code points; partial multi-byte
//	            sequences are preserved as-is.
//	-N NBYTES   Return after reading exactly NBYTES bytes (max 1 MiB),
//	            ignoring the delimiter. Last-set wins between -n and -N.
//	-t TIMEOUT  Time out after TIMEOUT seconds (decimal allowed).
//	            -t 0 / -t "" / -t . / -t + is a non-consuming poll:
//	            exit 0 if input is available, 1 otherwise. Ignored for
//	            regular-file stdin (bash's documented behaviour).
//	-h, --help  Print usage and exit.
//
// Field splitting:
//
//	Whitespace IFS chars (space, tab, newline) coalesce into a single
//	separator; non-whitespace IFS chars do not coalesce. With more
//	NAMEs than fields, missing names are assigned "". With more fields
//	than NAMEs, the last NAME absorbs the remainder verbatim.
//
// Backslash escapes (non-raw mode):
//
//	\<newline>       Line continuation — both bytes dropped.
//	\<X>             Literal X (escape suppresses delim and NUL on X).
//	\<NUL>           Both bytes dropped (NULs are always stripped).
//
// Exit codes:
//
//	0  Read completed (or -t 0 found input ready).
//	1  EOF before any byte / -t 0 with no input available / invalid
//	   identifier / value-too-large.
//	2  Variable access not available (find -exec / -execdir grandchild).
//	142  Positive -t TIMEOUT expired (128 + SIGALRM).
//
// Unsupported flags (vs. bash): -a (array), -s (silent), -u (read from
// FD), -e (readline), -i (initial text). These are not implemented
// because the underlying shell features (arrays, terminal-mode
// manipulation, FD redirection beyond pipe/file, line editor) are
// outside rshell's scope.
package read

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// MaxReadBytes bounds the number of input bytes that read will buffer
// for a single invocation. It also caps the values accepted for -n and
// -N at flag-parse time, so a script cannot ask read to allocate an
// arbitrary amount of memory.
const MaxReadBytes = 1 << 20 // 1 MiB

// Cmd is the read builtin command descriptor.
//
// No Help field is set: read registers its own --help flag (handled in
// registerFlags), and the convention enforced by
// builtins/tests/help.TestHelpFieldOnlyOnNoFlagsCommands is that
// flag-having commands surface help via --help so `help read` and
// `read --help` produce the same output.
var Cmd = builtins.Command{
	Name:        "read",
	Description: "read a line from standard input and assign to shell variables",
	MakeFlags:   registerFlags,
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// Bash documents `read [OPTION]... [NAME...]` and stops parsing flags
	// once the first NAME is seen — `read var -n 1` over `abc` assigns
	// var="abc" then errors on `-n` as an invalid identifier rather than
	// re-interpreting `-n 1` as a length flag. Disable pflag's default
	// interspersed-flag parsing so later -tokens remain NAMEs.
	fs.SetInterspersed(false)

	help := fs.BoolP("help", "h", false, "print usage and exit")
	raw := fs.BoolP("raw", "r", false, "do not interpret backslashes")
	prompt := fs.StringP("prompt", "p", "", "print PROMPT to stderr before reading")
	delim := fs.StringP("delim", "d", "", "use the first character of DELIM as the line terminator (empty = NUL)")
	timeoutStr := fs.StringP("timeout", "t", "", "time out after TIMEOUT seconds (decimal allowed)")

	// -n and -N must observe last-set-wins parse order to match bash:
	// `read -n 1 -N 2` reads 2 chars (-N is later) while `read -N 2 -n 1`
	// reads 1 char (-n is later). pflag tracks "was this flag set" via
	// Changed() but does not preserve relative parse order between two
	// flags, so we install a custom pflag.Value that records the order
	// each time Set() fires.
	nChars := -1
	nBytes := -1
	nCharsOrder := 0
	nBytesOrder := 0
	flagSeq := 0
	fs.VarP(&orderedIntValue{val: &nChars, order: &nCharsOrder, seq: &flagSeq}, "nchars", "n",
		fmt.Sprintf("return after reading at most NCHARS characters (max %d)", MaxReadBytes))
	fs.VarP(&orderedIntValue{val: &nBytes, order: &nBytesOrder, seq: &flagSeq}, "nbytes", "N",
		fmt.Sprintf("return after reading exactly NBYTES bytes (max %d), ignoring delimiters", MaxReadBytes))

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: read [OPTION]... [NAME...]\n")
			callCtx.Out("Read one line from standard input and assign each field to a shell\n")
			callCtx.Out("variable named by NAME, splitting on the characters in IFS. With no\n")
			callCtx.Out("NAME, the whole line is assigned to REPLY.\n")
			callCtx.Out("\n")
			callCtx.Out("Options:\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			callCtx.Out("\n")
			callCtx.Out("Field splitting: whitespace IFS chars (space, tab, newline) coalesce\n")
			callCtx.Out("into one separator; non-whitespace IFS chars do not. With more NAMEs\n")
			callCtx.Out("than fields, missing names are assigned \"\". With more fields than\n")
			callCtx.Out("NAMEs, the last NAME absorbs the remainder.\n")
			callCtx.Out("\n")
			callCtx.Out("Backslash escapes (non-raw mode): \\<newline> is a line continuation;\n")
			callCtx.Out("\\<X> for any other byte X is preserved as a literal X (suppressing\n")
			callCtx.Out("delim/NUL handling on X). NUL bytes are always stripped.\n")
			callCtx.Out("\n")
			callCtx.Out("Exit Status:\n")
			callCtx.Out("  0    Read completed (or -t 0 found input ready).\n")
			callCtx.Out("  1    EOF before any byte / -t 0 with no input / invalid identifier /\n")
			callCtx.Out("       value too large.\n")
			callCtx.Out("  2    Variable access not available (find -exec / -execdir grandchild).\n")
			callCtx.Out("  142  Positive -t TIMEOUT expired (128 + SIGALRM).\n")
			callCtx.Out("\n")
			callCtx.Out("Unsupported (vs. bash): -a (array), -s (silent), -u (read from FD),\n")
			callCtx.Out("-e (readline), -i (initial text).\n")
			return builtins.Result{}
		}
		return run(ctx, callCtx, args, runOpts{
			raw:         *raw,
			prompt:      *prompt,
			delim:       *delim,
			delimSet:    fs.Changed("delim"),
			nChars:      nChars,
			nBytes:      nBytes,
			nCharsOrder: nCharsOrder,
			nBytesOrder: nBytesOrder,
			timeoutStr:  *timeoutStr,
			timeoutSet:  fs.Changed("timeout"),
		})
	}
}

// orderedIntValue is a pflag.Value implementation that records, in
// addition to the parsed integer value, the relative parse-order
// position at which Set() was called. Sharing a single seq counter
// between two flags lets the handler determine which of `-n` / `-N`
// was specified last, which bash uses to break the tie when both
// length flags appear.
type orderedIntValue struct {
	val   *int
	order *int // updated to *seq on each Set; 0 means "not set"
	seq   *int // shared monotonic counter, incremented on each Set
}

func (v *orderedIntValue) String() string {
	if v.val == nil {
		return "-1"
	}
	return strconv.Itoa(*v.val)
}

func (v *orderedIntValue) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*v.val = n
	*v.seq++
	*v.order = *v.seq
	return nil
}

func (v *orderedIntValue) Type() string { return "int" }

type runOpts struct {
	raw         bool
	prompt      string
	delim       string
	delimSet    bool
	nChars      int
	nBytes      int
	nCharsOrder int
	nBytesOrder int
	timeoutStr  string
	timeoutSet  bool
}

func run(ctx context.Context, c *builtins.CallContext, args []string, opt runOpts) builtins.Result {
	if c.SetVar == nil || c.GetVar == nil {
		c.Errf("read: variable access is not available in this context\n")
		return builtins.Result{Code: 2}
	}

	// Resolve which of -n / -N applies: bash uses last-set-wins when both
	// are present. If neither was set both order values are 0; the equal
	// comparison falls through to the "no length limit" path.
	nFixedMode := opt.nBytesOrder > opt.nCharsOrder
	switch {
	case nFixedMode:
		if opt.nBytes < 0 {
			c.Errf("read: -N: count must be non-negative\n")
			return builtins.Result{Code: 1}
		}
		if opt.nBytes > MaxReadBytes {
			c.Errf("read: -N: count exceeds maximum of %d\n", MaxReadBytes)
			return builtins.Result{Code: 1}
		}
	case opt.nCharsOrder > 0:
		if opt.nChars < 0 {
			c.Errf("read: -n: count must be non-negative\n")
			return builtins.Result{Code: 1}
		}
		if opt.nChars > MaxReadBytes {
			c.Errf("read: -n: count exceeds maximum of %d\n", MaxReadBytes)
			return builtins.Result{Code: 1}
		}
	}

	// The delimiter is a single BYTE, not a rune: bash 5.2 treats `-d
	// DELIM` as "the first character of DELIM", but in practice it
	// scans the input byte-by-byte and stops at the first byte equal
	// to the first byte of DELIM. With a multi-byte UTF-8 DELIM (e.g.
	// `é` = 0xC3 0xA9), bash matches and consumes only the leading
	// byte (0xC3), leaving the trailing 0xA9 in the stream for
	// subsequent reads. Verified empirically:
	//
	//   printf 'a\xc3\xa9b' | { read -d $'\xc3' x; cat; }
	//     bash 5.2: x="a"; cat shows '\xa9 b'  (1 byte consumed for delim)
	//
	// A previous rune-based implementation consumed BOTH bytes of the
	// multi-byte char, which silently dropped one byte from any
	// downstream consumer. Use byte semantics to match bash.
	var delim byte = '\n'
	if opt.delimSet {
		if opt.delim == "" {
			delim = 0
		} else {
			delim = opt.delim[0]
		}
	}

	// Bash explicitly documents that `-t TIMEOUT` has no effect when
	// stdin is a regular file: regular-file reads don't block (the
	// kernel returns data or EOF immediately), so a timeout is
	// meaningless and bash skips its alarm path entirely. Setting up
	// context.WithTimeout here would cause `read -t 0.001 var <
	// bigfile` to spuriously return 142 with a partial assignment
	// when the timer expires while we're scanning a large line.
	// Detect the regular-file shape via fstat and skip the positive-
	// timeout setup; the syntactic validation of the -t value still
	// runs (matching bash, which rejects `read -t abc` even when
	// stdin is a regular file).
	stdinIsRegularFile := false
	if f, fok := c.Stdin.(*os.File); fok {
		if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
			stdinIsRegularFile = true
		}
	}

	// Resolve timeout. Bash treats -t 0 as a poll: returns 0 if input
	// is immediately available, non-zero otherwise, without waiting and
	// without consuming data. An explicit empty `-t ""` is also treated
	// as a zero-timeout poll (bash 5.2 verified empirically: `read -t
	// "" X` does not consume stdin or assign X). Positive timeouts wrap
	// the read in a context.WithTimeout (skipped for regular-file stdin
	// per the rule above). We also honour any deadline already on the
	// parent context (e.g. from interp.MaxExecutionTime or a CLI
	// --timeout) by propagating it to *os.File stdin via
	// SetReadDeadline so a blocking read syscall actually wakes up
	// when the run as a whole is cancelled, not only when -t fires.
	readCtx := ctx
	pollMode := false
	if opt.timeoutSet {
		if opt.timeoutStr == "" {
			pollMode = true
		} else {
			secs, ok := parseReadTimeout(opt.timeoutStr)
			if !ok {
				c.Errf("read: %s: invalid timeout specification\n", opt.timeoutStr)
				return builtins.Result{Code: 1}
			}
			switch {
			case secs == 0:
				pollMode = true
			case stdinIsRegularFile:
				// Bash: -t is ignored on regular files. Run the read
				// without a timeout-derived deadline.
			default:
				dur := time.Duration(secs * float64(time.Second))
				var cancel context.CancelFunc
				readCtx, cancel = context.WithTimeout(ctx, dur)
				defer cancel()
			}
		}
	}
	noNames := len(args) == 0
	names := args
	if noNames {
		names = []string{"REPLY"}
	}

	// Poll mode (-t 0 or -t "") returns immediately: 0 if input is
	// available, 1 otherwise. Performed before any other validation —
	// bash 5.2 does NOT validate identifier names in poll mode (e.g.
	// `printf 'a\n' | { read -t 0 1bad; read rest; }` returns the
	// poll status with no error and leaves rest=a). Done before the
	// prompt too, since poll mode never consumes input.
	//
	// Regular files are trivially "available" because their reads
	// don't block. Short-circuit to Code 0 here — this is required
	// for cross-platform correctness: pollAvailable on Windows uses
	// the SetReadDeadline+Read fallback (since the unix poll(2)
	// path doesn't exist), and SetReadDeadline rejects regular
	// files, so without this short-circuit `read -t 0 X < file`
	// would incorrectly return 1 on Windows for a deterministic
	// "always pollable" input shape.
	//
	// Run pollMode BEFORE the kernel-cancel block below: pollAvailable
	// returns instantly via poll(2) and never enters a blocking Read,
	// so the watchdog goroutine + SetReadDeadline syscalls would be
	// pure overhead.
	if pollMode {
		if stdinIsRegularFile {
			return builtins.Result{Code: 0}
		}
		return pollAvailable(c)
	}

	// Best-effort kernel-level cancellation for blocking stdin reads.
	// Works for *os.File pipes (the typical runner-supplied stdin shape).
	// Two layers:
	//
	//   1. Kernel deadline: when readCtx has a deadline (-t or
	//      MaxExecutionTime), install it via SetReadDeadline so the
	//      Read syscall wakes up on timeout. We pull the deadline from
	//      readCtx rather than computing one with time.Now to avoid a
	//      second time source — context.WithTimeout's internal clock,
	//      surfaced via Deadline(), is authoritative. When both -t and
	//      an inherited deadline apply, Deadline() returns the earlier
	//      of the two, which is the correct fused deadline.
	//
	//   2. Watchdog goroutine: any cancellable readCtx (deadline OR
	//      bare parent.Cancel()) is mirrored to the kernel-level
	//      deadline by setting it to a past time on ctx.Done(), forcing
	//      a blocked Read to unblock immediately even when no timeout
	//      is configured.
	//
	// SetReadDeadline returns "file type does not support deadline" for
	// regular files and /dev/null. Those types don't block on Read, so
	// the failure is harmless. For pollable types where Read CAN block
	// (TTYs, sockets, named pipes via the runtime poller, etc.) the
	// call generally succeeds. When kernelCancel is established, we
	// stay on the direct read path — which crucially does NOT prefetch
	// past the consumer's request and therefore preserves bytes for
	// subsequent reads on the shared stdin (e.g. `while read line`).
	// Only when neither the *os.File assertion nor SetReadDeadline
	// support holds do we fall back to the request-response goroutine
	// path in readInput.
	kernelCancel := false
	if c.Stdin != nil && readCtx.Done() != nil {
		if f, fok := c.Stdin.(*os.File); fok {
			// Probe SetReadDeadline support by attempting to clear.
			// This is a no-op for descriptors that already have no
			// deadline and a quick failure for unsupported types.
			if err := f.SetReadDeadline(time.Time{}); err == nil {
				if dl, ok := readCtx.Deadline(); ok {
					_ = f.SetReadDeadline(dl)
				}
				stop := make(chan struct{})
				watchdogDone := make(chan struct{})
				go func() {
					defer close(watchdogDone)
					select {
					case <-readCtx.Done():
						// Past time; any future blocked Read on f
						// returns ErrDeadlineExceeded immediately.
						_ = f.SetReadDeadline(time.Unix(1, 0))
					case <-stop:
					}
				}()
				defer func() {
					close(stop)
					<-watchdogDone
					_ = f.SetReadDeadline(time.Time{})
				}()
				kernelCancel = true
			}
		}
	}

	// Bash validates identifiers per-assignment but with one special
	// case: if the FIRST NAME is invalid, the command aborts WITHOUT
	// reading any input, leaving the stream untouched for the next
	// read. Any subsequent invalid NAME (positions 2..n) only fires
	// after the read has already happened — earlier valid names are
	// assigned, then the error stops further assignment. Verified
	// empirically against bash 5.2.0:
	//   `printf 'a\nb\n' | { read 1bad; read next; }` → next="a"
	//     (first NAME invalid, no read consumed).
	//   `printf 'a b c\n' | { read x 2bad z; }` → x="a", error,
	//     z unset; next read sees the second record.
	// Run the upfront leading-name check before readInput so the
	// stream stays intact in the leading-invalid case; defer the
	// rest to the assignment loop below.
	if !noNames && !validVarName(names[0]) {
		c.Errf("read: `%s': not a valid identifier\n", names[0])
		return builtins.Result{Code: 1}
	}

	// Bash only displays the -p prompt when stdin is an actual terminal,
	// not just any character device (so /dev/null does not trigger it).
	if opt.prompt != "" && stdinIsTerminal(c.Stdin) {
		fmt.Fprint(c.Stderr, opt.prompt)
	}

	// -N counts characters (not bytes) and ignores the delimiter. The
	// length-flag selection (last-set-wins between -n and -N) was
	// computed at the top of run() into nFixedMode.
	charLimit := -1
	switch {
	case nFixedMode:
		charLimit = opt.nBytes
	case opt.nCharsOrder > 0:
		charLimit = opt.nChars
	}
	// readInput needs the goroutine-based fallback only when the
	// context is cancellable AND we couldn't wire kernel-level
	// cancellation through SetReadDeadline above. Staying on the
	// direct Read path whenever possible is critical for correctness:
	// the goroutine path cannot avoid leaving a leaked goroutine
	// blocked inside Read on cancellation, which on a shared stdin
	// (the typical `while read line` shape) would consume bytes that
	// the next read iteration must observe. Kernel cancellation lets
	// us interrupt the blocked Read directly without prefetching.
	needsGoroutinePoll := readCtx.Done() != nil && !kernelCancel
	if c.Stdin == nil {
		// Nil stdin is treated as immediate EOF: fall through to the
		// assignment loop below so each NAME is cleared to "" before
		// the function returns 1, matching the EOF behaviour of bash
		// (and the explicit empty-line + EOF path further down).
		needsGoroutinePoll = false
	}
	line, eof, err := readInput(readCtx, c.Stdin, delim, opt.raw, charLimit, nFixedMode, needsGoroutinePoll)
	timedOut := false
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			// Bash convention for read -t timeout: 128 + SIGALRM (14) = 142.
			// Bash still assigns any partially-read data on timeout, so we
			// fall through to the assignment path and surface the 142 exit
			// code at the end. Empty-line + timeout is handled below.
			timedOut = true
		} else {
			c.Errf("read: %s\n", err)
			return builtins.Result{Code: 1}
		}
	}

	// On empty result + EOF or timeout, bash still assigns "" to every
	// requested NAME (clearing any prior value) before surfacing the
	// non-zero exit code. Falling through to the assignment loop with
	// the zero-valued `values` slice produces the same result for us.

	// Determine field values for each variable. Three cases, each matching
	// bash exactly:
	//   1. No NAMEs — raw line is assigned to REPLY, no IFS splitting and
	//      no leading/trailing IFS-whitespace stripping.
	//   2. -N mode (with NAMEs) — the entire read goes to the first NAME;
	//      remaining NAMEs are empty. IFS splitting is skipped.
	//   3. Default / -n mode — POSIX IFS field-splitting across NAMEs.
	values := make([]string, len(names))
	switch {
	case noNames:
		values[0] = line
	case nFixedMode:
		values[0] = line
	default:
		ifs := " \t\n"
		if v, ok := c.GetVar("IFS"); ok {
			ifs = v
		}
		fields := splitIFS(line, ifs, len(names))
		copy(values, fields)
	}

	// Assign each variable in order. Bash validates identifiers
	// per-assignment: when an invalid name is encountered, the error
	// fires immediately and remaining variables are left untouched, but
	// any earlier valid names have already been assigned (and the read
	// itself has already consumed input). Match that behaviour.
	for i, name := range names {
		if !validVarName(name) {
			c.Errf("read: `%s': not a valid identifier\n", name)
			return builtins.Result{Code: 1}
		}
		if err := c.SetVar(name, values[i]); err != nil {
			c.Errf("read: %s\n", err)
			// Total-storage exhaustion is the same script-aborting
			// resource-cap violation that AST-level assignment treats
			// as fatal. Surface it via Result.Exiting so the runner
			// stops the script rather than continuing past the cap.
			if errors.Is(err, builtins.ErrVarStorageExceeded) {
				return builtins.Result{Code: 1, Exiting: true}
			}
			return builtins.Result{Code: 1}
		}
	}

	if timedOut {
		return builtins.Result{Code: 142}
	}
	if eof {
		return builtins.Result{Code: 1}
	}
	return builtins.Result{}
}

// pollAvailable implements `read -t 0` semantics. Bash uses select(2)
// to report whether stdin is immediately readable without consuming
// any data; we use the platform-specific pollInputNonConsuming for
// the same effect on Unix.
//
// On platforms without a non-consuming poll syscall (e.g. Windows),
// we conservatively return "not available" — consuming a probe byte
// would silently drop input that subsequent reads would otherwise
// observe. Concretely, `printf x | { read -t 0 ready; read rest; }`
// must not lose the 'x' because the poll consumed it. Scripts that
// relied on the previous consume-based fallback would only see false
// negatives (we say "not available" when data actually is buffered),
// which causes them to fall through to a regular blocking read that
// then succeeds. A non-consuming Windows implementation (e.g. via
// PeekNamedPipe) is a follow-up.
//
// Returns:
//   - 0 — input is immediately available (data buffered to read).
//   - 1 — would block, no data buffered, or stdin is not pollable
//     (e.g. byte buffer, non-File reader, or any descriptor on a
//     platform without non-consuming poll). Bash uses exit code 1
//     for `-t 0` "no data available", reserving 142 (128+SIGALRM)
//     for the positive-timeout case where an alarm actually fired.
//
// Regular-file stdin is short-circuited to Code 0 by the caller in
// run() — pollAvailable here only handles pipes/sockets/TTYs/etc.
func pollAvailable(c *builtins.CallContext) builtins.Result {
	f, ok := c.Stdin.(*os.File)
	if !ok {
		return builtins.Result{Code: 1}
	}
	if avail, supported := pollInputNonConsuming(f.Fd()); supported {
		if avail {
			return builtins.Result{Code: 0}
		}
		return builtins.Result{Code: 1}
	}
	// Non-Unix fallback: report not-available rather than consume.
	return builtins.Result{Code: 1}
}

// stdinIsTerminal reports whether r is an *os.File whose descriptor
// refers to an actual terminal (TTY), as determined by the platform's
// isatty equivalent. Returns false for pipes, regular files, /dev/null,
// and other non-terminal character devices.
func stdinIsTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// parseReadTimeout parses bash's -t TIMEOUT syntax: a non-negative
// decimal number, optionally with a fractional part. Bash rejects
// scientific notation, NaN, Inf, and negatives — verified empirically
// by the "invalid timeout specification" error each of those produces.
// Returns the parsed seconds and ok=true on success, ok=false otherwise.
//
// The function also rejects values whose nanosecond representation
// would overflow time.Duration (a signed int64), so the conversion to
// time.Duration in the caller cannot wrap into a negative duration
// that would fire an immediate timeout.
func parseReadTimeout(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	// Bash accepts an optional leading `+` on -t TIMEOUT (verified
	// empirically: `read -t +1 v` and `read -t +0 v` both work in
	// bash 5.2). Strip it before the digit-only filter so the rest
	// of the parser doesn't have to special-case the sign.
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	if s == "" {
		// Bash treats bare `+` (the original input was just `+`,
		// nothing else) the same as the empty-string and `0`/`+0`
		// cases: a zero-timeout poll. Verified empirically against
		// bash 5.2.21:
		//   printf x | { read -t + v; read rest; }
		//     → rc=0, v="", rest="x"
		// Symmetric with the `.` / `+.` short-circuit below.
		return 0, true
	}
	// Accept only [0-9] and at most one '.', matching bash's parser
	// which excludes minus signs, exponents, and special tokens
	// (NaN, Inf, hex, etc.).
	seenDot := false
	seenDigit := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			seenDigit = true
		case r == '.' && !seenDot:
			seenDot = true
		default:
			return 0, false
		}
	}
	if !seenDigit {
		// Bash treats `.` and `+.` (lone dot, possibly with the
		// sign already stripped above) as a 0-timeout poll —
		// verified empirically against bash 5.2.0:
		//   echo x | { read -t . v; read rest; }   → v="" rest="x"
		// strconv.ParseFloat below would reject these tokens, so
		// short-circuit here. Anything without a digit AND without
		// a dot (i.e. empty after `+` strip) remains invalid.
		if seenDot {
			return 0, true
		}
		return 0, false
	}
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	// Defensive: NaN/Inf are excluded by the char filter above, but
	// guard anyway in case the float parser would accept some
	// surprising input.
	if math.IsNaN(secs) || math.IsInf(secs, 0) {
		return 0, false
	}
	// Bound by what time.Duration can represent without wrapping.
	maxSecs := float64(math.MaxInt64) / float64(time.Second)
	if secs > maxSecs {
		return 0, false
	}
	return secs, true
}

// validVarName reports whether name is a valid POSIX shell identifier:
// non-empty, starts with a letter or underscore, then letters, digits,
// or underscores.
func validVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		first := i == 0
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case !first && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// readInput reads from r byte-by-byte until one of:
//   - the delimiter byte is encountered (and ignoreDelim is false)
//   - charLimit characters have been read (when charLimit >= 0)
//   - EOF
//   - context cancellation or timeout
//   - the next byte would push the output buffer past MaxReadBytes
//
// The returned line excludes the trailing delimiter. eof reports whether
// the underlying reader reached EOF.
//
// Delimiter, NUL stripping, and backslash escapes all operate at the BYTE
// level, matching bash 5.2 — see the detailed comment on `delim` in run()
// for the bash-compat rationale. charLimit, in contrast, counts characters
// (UTF-8 code points): when -n N is given, we accumulate bytes into the
// buffer and increment the rune counter only when the trailing bytes form
// a complete rune (or when 4 bytes have piled up without completion, in
// which case bash counts the run as a single replacement char).
//
// In non-raw mode (raw=false), backslash sequences are interpreted:
//   - "\<newline>" is a line continuation: both bytes are dropped and
//     reading continues on the next physical line.
//   - "\<X>" for any other byte X reduces to X (the backslash is removed,
//     X is appended verbatim). Crucially, the escape suppresses delim
//     and NUL handling on X — `read -d , x` over `a\,b,c` produces
//     x="a,b" because the `,` after the backslash is preserved as a
//     literal byte.
//
// Reads happen one byte at a time. This is slow but correct: a buffered
// reader would consume bytes past the delimiter and prevent subsequent
// reads from the same underlying stream from observing them.
//
// The MaxReadBytes cap is checked just before each append so a value or
// line exactly at the cap (e.g. read -n 1048576 over 1 MiB of ASCII)
// succeeds; only a write that would exceed the cap is rejected.
func readInput(ctx context.Context, r io.Reader, delim byte, raw bool, charLimit int, ignoreDelim, useGoroutinePoll bool) (string, bool, error) {
	if r == nil {
		// No stdin (default runner configuration): treat as immediate
		// EOF. The caller's assignment loop will clear each requested
		// NAME to "", matching bash's behaviour when read hits EOF
		// before any data, and the empty-line + EOF return surfaces
		// exit code 1.
		return "", true, nil
	}
	var buf []byte

	// consumed counts every byte we read from r, including bytes that
	// are later discarded (line continuations, the backslash before an
	// escape, runes that exceed charLimit). It is the primary memory /
	// CPU bound for this builtin: an attacker-controlled stdin made of
	// repeated backslash-newline pairs would never grow buf and would
	// otherwise drain unbounded input. Capping consumed independently
	// of buf size guarantees the builtin always returns within
	// MaxReadBytes of input regardless of how much of that input ends
	// up assigned to a variable.
	//
	// The check uses `>` rather than `>=` so a record of exactly
	// MaxReadBytes data bytes can still observe its delimiter or EOF on
	// the next read attempt — only when consumed has already crossed
	// the cap (i.e. the previous byte put us at the boundary and we'd
	// be reading a non-terminator) does the error fire.
	consumed := 0

	var readByte func() (byte, error)
	if useGoroutinePoll {
		// Goroutine-based fallback used only when the context is
		// cancellable AND kernel-level cancellation via
		// SetReadDeadline was unavailable (e.g. non-*os.File stdin or
		// a specialised character device that rejects deadlines).
		//
		// IMPORTANT: this path uses a request-response pattern, not a
		// prefetch loop. A naive prefetch would call r.Read in a tight
		// loop and queue bytes ahead of the consumer; on a shared
		// stdin (the typical `while read line` shape) the producer
		// would consume bytes past the delimiter and the next read
		// iteration would see EOF or skip data. Instead, the producer
		// performs exactly one r.Read per request from the consumer
		// and sends the result on an unbuffered channel — bytes are
		// never read speculatively.
		//
		// Trade-off: if Read is stuck in the kernel when ctx fires,
		// the producer can't be force-cancelled. It will eventually
		// unblock when stdin produces data or closes; AT MOST ONE
		// byte is lost in that scenario (the byte the producer was
		// reading at the moment of cancellation). The leaked
		// goroutine exits via the stop signal once Read returns.
		type byteResult struct {
			b   byte
			err error
		}
		reqCh := make(chan struct{})
		resCh := make(chan byteResult)
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			var rbuf [1]byte
			// savedErr defers an error that arrived in the same Read
			// call as a successful byte (n=1 with err != nil — io.Reader
			// is allowed to return data and io.EOF together). The byte
			// is delivered this turn; the error surfaces on the next
			// readByte request, mirroring the direct-read path's
			// behaviour of returning the byte first and the EOF on the
			// subsequent Read.
			var savedErr error
			for {
				select {
				case <-stop:
					return
				case _, ok := <-reqCh:
					if !ok {
						return
					}
				}
				if savedErr != nil {
					select {
					case resCh <- byteResult{0, savedErr}:
					case <-stop:
					}
					return
				}
				var b byte
				var sendErr error
				gotData := false
				for {
					n, err := r.Read(rbuf[:])
					if n == 1 {
						b = rbuf[0]
						gotData = true
						if err != nil {
							savedErr = err
						}
						break
					}
					if err != nil {
						sendErr = err
						break
					}
					// n=0, err=nil per io.Reader contract: retry.
					// Watch both stop AND ctx.Done() — without the
					// ctx.Done() arm, a misbehaving reader that spins
					// (0, nil) indefinitely could pin a CPU until the
					// consumer's `defer close(stop)` fires (which only
					// happens after readInput returns, possibly after
					// the consumer has already given up via ctx
					// cancellation).
					select {
					case <-stop:
						return
					case <-ctx.Done():
						return
					default:
					}
				}
				select {
				case resCh <- byteResult{b, sendErr}:
				case <-stop:
					return
				}
				if !gotData && sendErr != nil {
					// Pure error result delivered; nothing more to do.
					return
				}
			}
		}()
		readByte = func() (byte, error) {
			if consumed > MaxReadBytes {
				return 0, fmt.Errorf("input exceeds maximum of %d bytes", MaxReadBytes)
			}
			select {
			case reqCh <- struct{}{}:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
			select {
			case res := <-resCh:
				if res.err != nil {
					return 0, res.err
				}
				consumed++
				return res.b, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	} else {
		// Direct path: either no deadline at all, or SetReadDeadline
		// has installed a kernel-level deadline that will surface as
		// os.ErrDeadlineExceeded from r.Read. ctx.Err() between
		// retries handles the io.Reader-contract case of n=0,err=nil.
		one := make([]byte, 1)
		readByte = func() (byte, error) {
			if consumed > MaxReadBytes {
				return 0, fmt.Errorf("input exceeds maximum of %d bytes", MaxReadBytes)
			}
			for {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
				n, err := r.Read(one)
				if n == 1 {
					consumed++
					return one[0], nil
				}
				if err != nil {
					return 0, err
				}
				// n == 0 && err == nil: io.Reader contract permits this; retry.
			}
		}
	}

	tryAppendByte := func(b byte) error {
		if len(buf)+1 > MaxReadBytes {
			return fmt.Errorf("input exceeds maximum of %d bytes", MaxReadBytes)
		}
		buf = append(buf, b)
		return nil
	}

	// Rune accounting for charLimit (`-n N` chars). Bash counts
	// characters in the user's locale; in a UTF-8 locale that means
	// code points. We track `lastRuneStart` as the index in `buf`
	// where the in-progress UTF-8 sequence began. Each appended byte
	// is tested against utf8.FullRune (or a 4-byte invalid-sequence
	// fallback that bash treats as a single replacement char) and
	// `runes` is incremented when the trailing bytes complete a rune.
	// Escaped bytes use the same accounting — the post-escape byte
	// becomes the next byte in the rune-progress stream.
	runes := 0
	lastRuneStart := 0
	maybeRuneComplete := func(b byte) {
		if b < utf8.RuneSelf {
			runes++
			lastRuneStart = len(buf)
			return
		}
		pending := buf[lastRuneStart:]
		if utf8.FullRune(pending) {
			runes++
			lastRuneStart = len(buf)
			return
		}
		// Bound at UTFMax bytes: a malformed multi-byte sequence
		// (overlong, missing continuation, lone start byte) gets
		// counted as a single replacement-char rune to keep the
		// rune-counter and charLimit comparison from running away.
		if len(pending) >= utf8.UTFMax {
			runes++
			lastRuneStart = len(buf)
		}
	}

	// inEscape tracks whether the previous byte was an unescaped
	// backslash awaiting its escapee. Hoisted out of the loop so
	// errors mid-escape (e.g., timeout after the backslash byte but
	// before the escapee arrived) can be reported without losing
	// state.
	inEscape := false
	for {
		if charLimit >= 0 && runes >= charLimit {
			return string(buf), false, nil
		}

		b, err := readByte()
		if errors.Is(err, io.EOF) {
			// Trailing backslash with no escapee: bash drops the
			// backslash and treats input as terminated. Any partial
			// UTF-8 bytes already in buf stay (the rune is left
			// truncated, matching bash byte-level semantics).
			return string(buf), true, nil
		}
		if err != nil {
			// Non-EOF error (timeout, ctx cancellation, transport
			// error). Any bytes already in buf stay — bash includes
			// already-consumed data in the assigned value before
			// surfacing the timeout exit code (142). The unconsumed
			// `b` (if any) is discarded; readByte signalled an
			// error rather than a successful byte.
			return string(buf), false, err
		}

		if inEscape {
			inEscape = false
			// Escape suppresses delim and NUL detection on the
			// escaped byte, mirroring bash: `\<delim>` is preserved
			// as a literal delim byte in the buffer, and `\<NUL>`
			// is dropped (NULs are always stripped regardless of
			// position).
			if b == '\n' {
				// Backslash-newline: line continuation. Drop both.
				continue
			}
			if b == 0 {
				continue
			}
			if aerr := tryAppendByte(b); aerr != nil {
				return string(buf), false, aerr
			}
			maybeRuneComplete(b)
			continue
		}

		// Byte-level delim check. With a multi-byte UTF-8 delim, only
		// the first byte (passed in here) matches; the trailing bytes
		// of the multi-byte sequence stay in the input stream for
		// subsequent reads. Matches bash 5.2 byte-scan semantics.
		if !ignoreDelim && b == delim {
			return string(buf), false, nil
		}

		// Bash strips embedded NUL bytes from the assigned value.
		// The only case where NUL is meaningful is as the delimiter
		// (`read -d ''`), handled by the delim check above. In any
		// other configuration — including -N mode with -d '' — bash
		// discards NULs without counting them toward charLimit.
		if b == 0 {
			continue
		}

		if !raw && b == '\\' {
			inEscape = true
			continue
		}

		if aerr := tryAppendByte(b); aerr != nil {
			return string(buf), false, aerr
		}
		maybeRuneComplete(b)
	}
}

// splitIFS splits s into exactly n fields using POSIX read-style field
// splitting based on IFS:
//
//   - Whitespace IFS chars (space, tab, newline) coalesce: a run of
//     consecutive IFS-whitespace counts as one separator.
//   - Non-whitespace IFS chars do not coalesce: each occurrence introduces
//     a separate (possibly empty) field.
//   - Leading and trailing IFS-whitespace is stripped from the input.
//   - When the input has more fields than n, the n-th field absorbs the
//     remainder of the input verbatim, with only trailing IFS-whitespace
//     stripped.
//   - A single trailing non-whitespace IFS character is treated as the
//     start of an empty trailing field that bash silently discards. We
//     recognise this when the absorbed last field contains exactly one
//     non-whitespace IFS character and that character is at the end —
//     in which case it is stripped. With two or more non-whitespace IFS
//     characters in the last field, none are stripped (matches bash:
//     `IFS=: read a b` of `a:b:` → b="b"; of `a:b::` → b="b::").
//   - When the input has fewer fields than n, the missing fields are "".
func splitIFS(s, ifs string, n int) []string {
	if n <= 0 {
		return nil
	}
	isWS := func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }
	inIFS := func(r rune) bool { return strings.ContainsRune(ifs, r) }
	inIFSWS := func(r rune) bool { return inIFS(r) && isWS(r) }
	inIFSNonWS := func(r rune) bool { return inIFS(r) && !isWS(r) }

	s = trimLeadingFunc(s, inIFSWS)

	fields := make([]string, 0, n)
	for len(fields) < n-1 && s != "" {
		// Read one field: until the first IFS character.
		i := 0
		for i < len(s) {
			r, size := utf8.DecodeRuneInString(s[i:])
			if inIFS(r) {
				break
			}
			i += size
		}
		fields = append(fields, s[:i])
		s = s[i:]

		// Consume one separator: optional ifs-ws + at most one ifs-non-ws + optional ifs-ws.
		s = trimLeadingFunc(s, inIFSWS)
		if s != "" {
			r, size := utf8.DecodeRuneInString(s)
			if inIFSNonWS(r) {
				s = s[size:]
				s = trimLeadingFunc(s, inIFSWS)
			}
		}
	}

	// Last field absorbs the remainder with trailing IFS-whitespace stripped.
	s = trimTrailingFunc(s, inIFSWS)

	// Strip a lone trailing non-ws IFS character only when removing it
	// (along with any orphan IFS-whitespace between the data and the
	// trailing separator) leaves a single-field absorbed remainder with
	// NO IFS characters at all. Bash treats the trailing non-ws IFS
	// char as the start of an empty trailing field that gets dropped
	// only in this single-field shape; with multiple internal IFS
	// chars (ws or non-ws) the absorbed remainder represents data
	// from multiple fields and is preserved verbatim, including the
	// trailing delim. Verified empirically against bash 5.2:
	//
	//   IFS=' ,' read a    <<< 'a,'      → a="a"   (strip)
	//   IFS=' ,' read a    <<< 'a ,'     → a="a"   (strip; trailing ws collapses too)
	//   IFS=' ,' read a    <<< 'b a,'    → a="b a," (preserve; internal ' ' is IFS-ws)
	//   IFS=' ,' read a    <<< 'b,a,'    → a="b,a," (preserve; multiple non-ws IFS)
	//   IFS=':'  read a    <<< 'b a:'    → a="b a"  (strip; ' ' is NOT IFS)
	//   IFS=' :' read a b  <<< ' a : b : ' → b="b" (strip; absorbed "b" has no IFS)
	//   IFS=' ,' read a b  <<< 'a b c,'  → b="b c," (preserve; absorbed has internal ' ')
	if r, size := utf8.DecodeLastRuneInString(s); inIFSNonWS(r) {
		candidate := trimTrailingFunc(s[:len(s)-size], inIFSWS)
		hasIFS := false
		for _, cr := range candidate {
			if inIFS(cr) {
				hasIFS = true
				break
			}
		}
		if !hasIFS {
			s = candidate
		}
	}

	fields = append(fields, s)

	for len(fields) < n {
		fields = append(fields, "")
	}
	return fields
}

func trimLeadingFunc(s string, pred func(rune) bool) string {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !pred(r) {
			return s[i:]
		}
		i += size
	}
	return ""
}

func trimTrailingFunc(s string, pred func(rune) bool) string {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if !pred(r) {
			return s
		}
		s = s[:len(s)-size]
	}
	return s
}
