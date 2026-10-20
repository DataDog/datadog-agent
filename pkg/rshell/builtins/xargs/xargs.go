// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package xargs implements the xargs builtin command.
//
// xargs — build and execute commands from standard input
//
// Usage: xargs [OPTION]... [COMMAND [INITIAL-ARGS]...]
//
// Reads items from standard input (or from FILE with -a), separated by
// whitespace (or by NUL with -0, or by a custom delimiter with -d) and
// executes COMMAND (default: echo) with the items appended as arguments.
//
// Items may be batched into one invocation per N items (-n), per N input
// lines (-L), or up to a maximum command-line length (-s).
//
// Accepted flags:
//
//	-0, --null
//	    Items are separated by a NUL character. Quoting and backslash
//	    escapes are not handled — every character is literal.
//
//	-a, --arg-file=FILE
//	    Read items from FILE instead of standard input. Subject to the
//	    AllowedPaths sandbox.
//
//	-d, --delimiter=DELIM
//	    Use DELIM as the single-byte item separator. Recognised escape
//	    forms: \n, \t, \r, \\, \0.
//
//	-E EOF-STR
//	    Treat an unquoted, unescaped occurrence of EOF-STR as a logical
//	    end-of-input. Has no effect if -0 or -d is in use.
//
//	-I REPLSTR
//	    Insert input as the value of REPLSTR in each COMMAND argument.
//	    Implies "one input item per command" and "-L 1" semantics, and
//	    switches tokenisation to newline-only (matches GNU xargs).
//
//	-L NUMBER
//	    Use up to NUMBER non-empty input lines per command invocation.
//
//	-n, --max-args=N
//	    Use at most N arguments per command invocation.
//
//	-r, --no-run-if-empty
//	    Do not execute COMMAND if the input contains no items.
//
//	-s, --max-chars=N
//	    Limit a single command line to N characters.
//
//	-t, --verbose
//	    Print the resolved command line on stderr before each invocation.
//
//	-x, --exit
//	    With -n or -L, abort if -s is too small to fit a single batch.
//
//	--help
//	    Print usage to stdout and exit 0.
//
// Exit codes (POSIX):
//
//	0    success
//	1    syntax / usage error or sub-command not allowed; also returned
//	     when the context is cancelled mid-run.
//	123  any sub-command exited 1..125
//	124  any sub-command exited 255 (xargs stops immediately)
//	125  sub-command failed to start
//
// Sandbox notes:
//
//	xargs only invokes other registered builtins through CallContext.RunCommand
//	and respects CallContext.CommandAllowed. There is no path to host
//	binaries; the GTFOBins shell-escape technique
//	"xargs -a /dev/null /bin/sh" is rejected because /bin/sh is not a
//	registered builtin. The -a FILE source goes through CallContext.OpenFile
//	so AllowedPaths still applies.
//
// Memory safety:
//
//	A per-token cap (MaxTokenBytes, 1 MiB) bounds any single argument and
//	prevents unbounded buffering on infinite streams. Read chunks are 64
//	KiB. ctx.Err() is checked between chunks, periodically inside per-byte
//	loops, and before each sub-command call so cancellation propagates
//	promptly even when the input stream is infinite (e.g. /dev/zero).
package xargs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the xargs builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "xargs",
	Description: "build and execute commands from standard input",
	MakeFlags:   registerFlags,
}

// Resource caps. These keep the builtin defensive against malicious or
// runaway input regardless of the user-supplied -n / -L / -s values.
const (
	// MaxTokenBytes is the largest single input item we will buffer.
	// A single argument longer than this is treated as an error.
	MaxTokenBytes = 1 << 20 // 1 MiB

	// DefaultMaxChars is the default command-line size when -s is unset.
	// Smaller than POSIX ARG_MAX (128 KiB on most Linux systems) — defensive.
	DefaultMaxChars = 128 * 1024

	// HardMaxChars is the upper clamp applied to user-provided -s values.
	HardMaxChars = 1 << 20 // 1 MiB

	// HardMaxArgs is the upper clamp on -n / -L values.
	HardMaxArgs = 1 << 20

	// readChunk is the per-Read size for bufio scanning.
	readChunk = 64 * 1024

	// ctxCheckEvery is the per-byte cadence at which token loops poll
	// context cancellation. Chosen so a 1-MiB-token DoS sees ≥ 256 polls.
	ctxCheckEvery = 4096

	// subCmdFatalCode is the sub-command exit value that triggers
	// xargs's "abort everything" code path (POSIX).
	subCmdFatalCode uint8 = 255
)

// emptyChildStdin is the POSIX-style /dev/null analogue passed as the child
// command's stdin via RunCommandWithStdin. Reuse one stateless reader across
// all invocations: the underlying buffer is nil, so Read always returns
// (0, io.EOF) and there is no offset state to corrupt across calls.
var emptyChildStdin io.Reader = bytes.NewReader(nil)

const (

	// Exit codes per POSIX xargs.
	exitOK             uint8 = 0
	exitUsage          uint8 = 1
	exitSubCmdFailed   uint8 = 123
	exitSubCmd255      uint8 = 124
	exitSubCmdNotStart uint8 = 125
)

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// Stop flag parsing at the first positional so flags after the COMMAND
	// name (e.g. `xargs echo -n hello`) are passed through to the sub-command
	// rather than re-interpreted by xargs.
	fs.SetInterspersed(false)

	help := fs.BoolP("help", "h", false, "print usage and exit")
	argFile := fs.StringP("arg-file", "a", "", "read items from FILE instead of stdin")
	eofStr := fs.StringP("eof", "E", "", "treat EOF-STR as a logical end-of-input marker")

	// -0 / -d are mutually exclusive (GNU last-wins). Track which was set
	// most recently and honor only that one in buildOptions.
	var sep sepTracker
	null := new(bool)
	fs.VarP(&trackedBool{p: null, key: sepNull, t: &sep}, "null", "0", "input items are separated by a NUL character")
	// Mark as a no-argument boolean flag so `-0` (no value) is accepted.
	fs.Lookup("null").NoOptDefVal = "true"
	delim := new(string)
	fs.VarP(&trackedString2{p: delim, key: sepDelim, t: &sep}, "delimiter", "d", "use DELIM as the single-byte item separator")
	noRunIfEmpty := fs.BoolP("no-run-if-empty", "r", false, "do not run command if input is empty")
	maxChars := fs.IntP("max-chars", "s", 0, "limit a single command line to N characters")
	verbose := fs.BoolP("verbose", "t", false, "print the command line on stderr before running")
	exitOnSize := fs.BoolP("exit", "x", false, "abort if -s is too small to fit a -n/-L batch")

	// -n / -L / -I are mutually exclusive (GNU "last-wins" + warning). A
	// single tracker lets us tell which of the three was set most recently.
	var batch batchTracker
	maxLines := new(int)
	fs.VarP(&trackedInt{p: maxLines, key: batchL, t: &batch}, "max-lines", "L", "use at most NUMBER non-empty input lines per command")
	maxArgs := new(int)
	fs.VarP(&trackedInt{p: maxArgs, key: batchN, t: &batch}, "max-args", "n", "use at most N arguments per command invocation")
	replStr := new(string)
	fs.VarP(&trackedString{p: replStr, key: batchI, t: &batch}, "replace", "I", "insert input as the value of REPLSTR in each argument")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: xargs [OPTION]... [COMMAND [INITIAL-ARGS]...]\n")
			callCtx.Out("Build and execute commands from standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		// Apply GNU's mutex: only the last-set among -n / -L / -I is
		// honored. Emit the matching "options X and Y are mutually
		// exclusive" warning to stderr for each ignored option so the
		// stderr stream stays bash-compatible.
		nSet := fs.Changed("max-args") && batch.last == batchN
		lSet := fs.Changed("max-lines") && batch.last == batchL
		iSet := fs.Changed("replace") && batch.last == batchI
		warnMutex(callCtx, fs, batch.last)
		effectiveMaxArgs := 0
		if nSet {
			effectiveMaxArgs = *maxArgs
		}
		effectiveMaxLines := 0
		if lSet {
			effectiveMaxLines = *maxLines
		}
		effectiveReplStr := ""
		if iSet {
			effectiveReplStr = *replStr
		}

		// Apply GNU last-wins for -0 / -d as well.
		nullSet := fs.Changed("null") && sep.last == sepNull && *null
		delimSet := fs.Changed("delimiter") && sep.last == sepDelim
		effectiveNull := nullSet
		effectiveDelim := ""
		if delimSet {
			effectiveDelim = *delim
		}
		opts, errMsg := buildOptions(fs, effectiveNull, *argFile, effectiveDelim, *eofStr, effectiveReplStr,
			effectiveMaxLines, effectiveMaxArgs, *noRunIfEmpty, *maxChars, *verbose, *exitOnSize, args,
			nSet, lSet, iSet, delimSet)
		if errMsg != "" {
			callCtx.Errf("xargs: %s\n", errMsg)
			return builtins.Result{Code: exitUsage}
		}

		return runXargs(ctx, callCtx, opts)
	}
}

// warnMutex emits GNU-style "options X and Y are mutually exclusive,
// ignoring previous X value" stderr warnings for each ignored flag in
// the -n / -L / -I mutex group. GNU findutils 4.9.0 renders the
// winning option with both long and short names (or short only for
// -L) and the ignored option with the long name only — preserved here
// so scenarios can stay byte-for-byte bash-compatible:
//
//	-n winner: "--max-args/-n",     loser: "--max-args"
//	-L winner: "-L",                loser: "--max-lines"
//	-I winner: "--replace/-I/-i",   loser: "--replace"
func warnMutex(callCtx *builtins.CallContext, fs *builtins.FlagSet, winner batchKey) {
	type spec struct {
		key         batchKey
		flag        string
		winnerLabel string
		loserLabel  string
	}
	all := []spec{
		{batchN, "max-args", "--max-args/-n", "--max-args"},
		{batchL, "max-lines", "-L", "--max-lines"},
		{batchI, "replace", "--replace/-I/-i", "--replace"},
	}
	winnerLabel := ""
	for _, s := range all {
		if s.key == winner {
			winnerLabel = s.winnerLabel
			break
		}
	}
	if winnerLabel == "" {
		return
	}
	for _, s := range all {
		if s.key == winner {
			continue
		}
		if !fs.Changed(s.flag) {
			continue
		}
		callCtx.Errf("xargs: warning: options %s and %s are mutually exclusive, ignoring previous %s value\n",
			s.loserLabel, winnerLabel, s.loserLabel)
	}
}

// batchKey identifies which of the mutex-grouped flags (-n / -L / -I) was
// last set on the command line.
type batchKey int

const (
	batchNone batchKey = iota
	batchN
	batchL
	batchI
)

// batchTracker records the last-set among -n / -L / -I so registerFlags can
// honor GNU xargs's "last-wins" semantics for these mutually-exclusive flags.
type batchTracker struct{ last batchKey }

// trackedInt wraps a *int target with a side-effect that updates the shared
// batchTracker on Set. Used as the pflag.Value for -n / -L.
type trackedInt struct {
	p   *int
	key batchKey
	t   *batchTracker
}

func (b *trackedInt) String() string { return fmt.Sprintf("%d", *b.p) }
func (b *trackedInt) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid integer %q: %w", s, err)
	}
	*b.p = v
	b.t.last = b.key
	return nil
}
func (b *trackedInt) Type() string { return "int" }

// trackedString wraps a *string target for -I.
type trackedString struct {
	p   *string
	key batchKey
	t   *batchTracker
}

func (b *trackedString) String() string { return *b.p }
func (b *trackedString) Set(s string) error {
	*b.p = s
	b.t.last = b.key
	return nil
}
func (b *trackedString) Type() string { return "string" }

// sepKey identifies which input-mode flag (-0 or -d) was last set.
type sepKey int

const (
	sepNone sepKey = iota
	sepNull
	sepDelim
)

// sepTracker records the last-set among -0 / -d so registerFlags can
// honor GNU xargs's "last-wins" semantics for these mutex separator flags.
type sepTracker struct{ last sepKey }

// trackedBool wraps a *bool target with an order-tracking side effect.
type trackedBool struct {
	p   *bool
	key sepKey
	t   *sepTracker
}

func (b *trackedBool) String() string { return strconv.FormatBool(*b.p) }
func (b *trackedBool) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*b.p = v
	if v {
		b.t.last = b.key
	}
	return nil
}
func (b *trackedBool) Type() string     { return "bool" }
func (b *trackedBool) IsBoolFlag() bool { return true }

// trackedString2 wraps a *string target for -d, sharing the sep tracker.
type trackedString2 struct {
	p   *string
	key sepKey
	t   *sepTracker
}

func (b *trackedString2) String() string { return *b.p }
func (b *trackedString2) Set(s string) error {
	*b.p = s
	b.t.last = b.key
	return nil
}
func (b *trackedString2) Type() string { return "string" }

// modeKind classifies how items are tokenised from the input.
type modeKind int

const (
	modeWhitespace modeKind = iota // POSIX-style whitespace + quoting
	modeNull                       // NUL-separated, no quoting
	modeDelim                      // single custom byte, no quoting
	modeLine                       // newline-separated, no quoting (used by -I)
)

// options holds the resolved configuration for a single xargs invocation.
//
// The presence of -n / -L / -s / -I / -a / -E is encoded in the value
// itself: a non-zero int (-n/-L) or non-default mode/string indicates the
// flag was set. We avoid parallel "useX bool" fields because they are a
// repeat of the value's own zero/non-zero state.
type options struct {
	mode    modeKind
	delim   byte   // valid only when mode == modeDelim
	argFile string // empty == not set
	eofStr  string // empty == not set

	replStr    string // empty == -I not set
	maxLines   int    // 0 == -L not set
	maxArgs    int    // 0 == -n not set
	maxChars   int    // always set (defaulted to DefaultMaxChars)
	noRunEmpty bool
	verbose    bool
	exitOnSize bool

	cmdName     string
	initialArgs []string
}

// useReplace, useMaxLines, useMaxArgs are derived predicates kept as
// methods to make call sites read clearly without storing redundant bools.
func (o *options) useReplace() bool  { return o.replStr != "" }
func (o *options) useMaxLines() bool { return o.maxLines > 0 }
func (o *options) useMaxArgs() bool  { return o.maxArgs > 0 }

// buildOptions validates the parsed flag values and resolves the command
// to be executed. It returns a non-empty error string on validation failure.
//
// nSet / lSet / iSet reflect whether -n / -L / -I were the last-specified
// among that mutex group on the command line. Only the winner contributes
// to the resolved options; the others are dropped silently.
//
// delimSet is the analogue for -d (with `null` already reflecting the
// last-wins resolution for -0 vs -d).
func buildOptions(fs *builtins.FlagSet, null bool, argFile, delim, eofStr, replStr string,
	maxLines, maxArgs int, noRunEmpty bool, maxChars int,
	verbose, exitOnSize bool, args []string,
	nSet, lSet, iSet, delimSet bool) (options, string) {

	o := options{
		mode:       modeWhitespace,
		eofStr:     eofStr,
		noRunEmpty: noRunEmpty,
		verbose:    verbose,
		exitOnSize: exitOnSize,
	}

	if null {
		o.mode = modeNull
	} else if delimSet {
		b, err := decodeDelim(delim)
		if err != nil {
			return o, fmt.Sprintf("invalid delimiter: %s", err)
		}
		o.mode = modeDelim
		o.delim = b
	}

	if fs.Changed("arg-file") {
		if argFile == "" {
			return o, "argument file path is empty"
		}
		o.argFile = argFile
	}

	if iSet {
		if replStr == "" {
			return o, "replace string must be non-empty"
		}
		o.replStr = replStr
	}

	if lSet {
		if msg := validatePositive("L", maxLines, HardMaxArgs); msg != "" {
			return o, msg
		}
		o.maxLines = maxLines
	}

	if nSet {
		if msg := validatePositive("n", maxArgs, HardMaxArgs); msg != "" {
			return o, msg
		}
		o.maxArgs = maxArgs
	}

	if fs.Changed("max-chars") {
		if maxChars <= 0 {
			return o, fmt.Sprintf("invalid -s value: %d", maxChars)
		}
		if maxChars > HardMaxChars {
			maxChars = HardMaxChars
		}
		o.maxChars = maxChars
	} else {
		o.maxChars = DefaultMaxChars
	}

	// -I forces single-arg-per-batch and per-line invocation, and switches
	// tokenisation to newline-only (matches GNU xargs: "unquoted blanks do
	// not terminate input items; instead the separator is the newline").
	// This overrides any user-supplied -n / -L values.
	if o.useReplace() {
		o.maxArgs = 1
		o.maxLines = 1
		// Only override mode when the user did not request -0 or -d.
		if !null && !delimSet {
			o.mode = modeLine
		}
	}

	if len(args) == 0 {
		o.cmdName = "echo"
	} else {
		o.cmdName = args[0]
		if len(args) > 1 {
			o.initialArgs = append([]string(nil), args[1:]...)
		}
	}

	// EOF-STR is honoured in whitespace and -I (line) modes; GNU xargs
	// only ignores -E when -0 or -d is in use (a delimiter explicitly
	// disables quote/escape processing, so an EOF-STR token cannot be
	// recognised reliably). -I keeps quote/escape semantics, so its
	// per-line tokens can still be matched against EOF-STR.
	if o.mode == modeNull || o.mode == modeDelim {
		o.eofStr = ""
	}

	return o, ""
}

// validatePositive returns "" when v is in range (0, max], else an error
// message suitable for surfacing as "xargs: <msg>".
func validatePositive(name string, v, max int) string {
	if v <= 0 || v > max {
		return fmt.Sprintf("invalid -%s value: %d", name, v)
	}
	return ""
}

// decodeDelim turns the -d argument into a single byte. Recognised escapes
// match the GNU xargs manual: \a, \b, \f, \n, \r, \t, \v, \\, \0, plus
// hex (\xNN) and octal (\NNN) numeric escapes. Multi-character delimiters
// other than these escapes are rejected (POSIX leaves the behaviour for
// multi-byte delimiters undefined; we choose explicit rejection).
func decodeDelim(s string) (byte, error) {
	switch s {
	case "":
		return 0, errors.New("empty delimiter")
	case `\a`:
		return '\a', nil
	case `\b`:
		return '\b', nil
	case `\f`:
		return '\f', nil
	case `\n`:
		return '\n', nil
	case `\r`:
		return '\r', nil
	case `\t`:
		return '\t', nil
	case `\v`:
		return '\v', nil
	case `\\`:
		return '\\', nil
	case `\0`:
		return 0, nil
	}
	if len(s) == 1 {
		return s[0], nil
	}
	// GNU xargs additionally accepts hex (`\xNN` / `\xN`) and octal
	// (`\NNN` / `\NN` / `\N`) escapes — the raw byte value of the
	// resulting code point is used as the single-byte delimiter.
	if len(s) >= 3 && s[0] == '\\' && (s[1] == 'x' || s[1] == 'X') {
		v, err := strconv.ParseUint(s[2:], 16, 8)
		if err == nil {
			return byte(v), nil
		}
	}
	if len(s) >= 2 && s[0] == '\\' && s[1] >= '0' && s[1] <= '7' {
		v, err := strconv.ParseUint(s[1:], 8, 8)
		if err == nil {
			return byte(v), nil
		}
	}
	return 0, fmt.Errorf("delimiter must be a single character: %q", s)
}

// runXargs reads items from the configured source, batches them, and invokes
// the resolved command for each batch via callCtx.RunCommand.
func runXargs(ctx context.Context, callCtx *builtins.CallContext, o options) builtins.Result {
	rc, err := openInput(ctx, callCtx, o)
	if err != nil {
		callCtx.Errf("xargs: %s\n", err.Error())
		return builtins.Result{Code: exitUsage}
	}
	if rc == nil {
		// No stdin available and no -a file given. POSIX xargs reads from
		// /dev/null in that case, which produces zero items.
		return finishEmpty(ctx, callCtx, o, exitOK)
	}
	defer rc.Close()

	tok := newTokenizer(rc, o)
	finalCode := exitOK
	totalItems := 0
	tokErr := false
	var batch []string
	batchLines := 0
	usedChars := commandLineLen(o, nil) // running command-line length

	flush := func() bool {
		code, stop := invokeCommand(ctx, callCtx, o, batch)
		batch = batch[:0]
		batchLines = 0
		usedChars = commandLineLen(o, nil)
		if code > finalCode {
			finalCode = code
		}
		return stop
	}

	for {
		if err := ctx.Err(); err != nil {
			return builtins.Result{Code: exitUsage}
		}
		item, endedLine, more, err := tok.next(ctx)
		if err != nil {
			callCtx.Errf("xargs: %s\n", err.Error())
			if finalCode < exitUsage {
				finalCode = exitUsage
			}
			tokErr = true
			break
		}
		// Emit GNU's "NUL character occurred" warning to stderr exactly
		// once per invocation when the tokenizer rejected NUL bytes in
		// a non-NUL mode. Polled here so the warning is emitted as soon
		// as the offending record is processed (or skipped) rather than
		// at end-of-input.
		if tok.nulSeen && !tok.nulWarned {
			tok.nulWarned = true
			callCtx.Errf("xargs: WARNING: a NUL character occurred in the input.  " +
				"It cannot be passed through in the argument list.  Did you mean to use the --null option?\n")
		}
		if !more {
			break
		}

		// Will this item still fit in the current batch's -s budget? Skip
		// this preflight when -I is active: the resolved command line after
		// REPLSTR substitution can be much shorter than the raw template +
		// item (e.g. a long REPLSTR replaced by a short item). The
		// post-substitution length check inside `invokeCommand` handles -s
		// correctly for the -I path.
		add := len(item) + 1
		if !o.useReplace() && usedChars+add > o.maxChars {
			// -L treats each input line as an indivisible batch (GNU
			// "implies -x"); -n with -x also requires the full batch to
			// fit. In either case we abort instead of silently splitting.
			lImpliedExit := o.useMaxLines()
			explicitExit := o.exitOnSize && o.useMaxArgs()
			if (lImpliedExit || explicitExit) && len(batch) > 0 {
				callCtx.Errf("xargs: argument list too long\n")
				return builtins.Result{Code: exitUsage}
			}
			// Otherwise, try to make room by flushing any pending batch.
			if len(batch) > 0 {
				if flush() {
					return builtins.Result{Code: finalCode}
				}
			}
			// If a single item still can't fit alongside the command name,
			// GNU xargs always exits 1 (regardless of -x).
			if usedChars+add > o.maxChars {
				callCtx.Errf("xargs: argument line too long\n")
				return builtins.Result{Code: exitUsage}
			}
		}

		batch = append(batch, item)
		usedChars += add
		if endedLine {
			batchLines++
		}
		totalItems++

		shouldFlush := (o.useMaxArgs() && len(batch) >= o.maxArgs) ||
			(o.useMaxLines() && batchLines >= o.maxLines)
		if shouldFlush {
			if flush() {
				return builtins.Result{Code: finalCode}
			}
		}
	}

	if len(batch) > 0 {
		if flush() {
			return builtins.Result{Code: finalCode}
		}
	}

	if totalItems == 0 && !tokErr {
		return finishEmpty(ctx, callCtx, o, finalCode)
	}

	return builtins.Result{Code: finalCode}
}

// finishEmpty handles the "no items consumed" case at end-of-input.
func finishEmpty(ctx context.Context, callCtx *builtins.CallContext, o options, prior uint8) builtins.Result {
	if o.noRunEmpty {
		return builtins.Result{Code: prior}
	}
	// -I implies "one invocation per input item"; with zero items, GNU xargs
	// runs nothing. Without -I we fall back to a single invocation with no
	// extra args (POSIX default).
	if o.useReplace() {
		return builtins.Result{Code: prior}
	}
	// The command name + initial args alone must still fit within -s; GNU
	// xargs aborts before invoking when even the bare command exceeds the
	// size budget (e.g. `xargs -s 1 echo`).
	if commandLineLen(o, nil) > o.maxChars {
		callCtx.Errf("xargs: argument line too long\n")
		if exitUsage > prior {
			prior = exitUsage
		}
		return builtins.Result{Code: prior}
	}
	code, _ := invokeCommand(ctx, callCtx, o, nil)
	if code > prior {
		prior = code
	}
	return builtins.Result{Code: prior}
}

// commandLineLen estimates the length of the command line that would be
// formed by COMMAND + INITIAL-ARGS + batch + a NUL terminator per arg.
// Approximation matches the way GNU xargs accounts for arguments
// (each token counted by length + 1 for the separator/terminator).
func commandLineLen(o options, batch []string) int {
	total := len(o.cmdName) + 1
	for _, a := range o.initialArgs {
		total += len(a) + 1
	}
	for _, a := range batch {
		total += len(a) + 1
	}
	return total
}

// invokeCommand dispatches a single batch through callCtx.RunCommand.
// Returns the final exit code we should report and a stop flag indicating
// the caller should not invoke further batches (sub-command exit 255 or
// other fatal condition).
func invokeCommand(ctx context.Context, callCtx *builtins.CallContext, o options, batch []string) (uint8, bool) {
	if err := ctx.Err(); err != nil {
		return exitUsage, true
	}

	finalCmd, finalArgs := resolveCmd(o, batch)

	// With -I the template may repeat the marker, so the resolved command
	// line can exceed -s even though the raw template + raw item fit.
	// GNU xargs aborts in that case with "command too long".
	if o.useReplace() {
		resolved := len(finalCmd) + 1
		for _, a := range finalArgs {
			resolved += len(a) + 1
		}
		if resolved > o.maxChars {
			callCtx.Errf("xargs: argument line too long\n")
			return exitUsage, true
		}
	}

	if o.verbose {
		printVerbose(callCtx, finalCmd, finalArgs)
	}

	if callCtx.RunCommand == nil {
		callCtx.Errf("xargs: command execution not available\n")
		return exitSubCmdNotStart, true
	}
	if callCtx.CommandAllowed != nil && !callCtx.CommandAllowed(finalCmd) {
		callCtx.Errf("xargs: %s: command not allowed\n", finalCmd)
		return exitSubCmdNotStart, true
	}

	dir := ""
	if callCtx.WorkDir != nil {
		dir = callCtx.WorkDir()
	}

	// POSIX/GNU xargs only redirects the child's stdin from /dev/null when
	// xargs itself is reading items from the parent's stdin. With -a FILE,
	// items come from FILE and the parent's stdin remains available to the
	// child (e.g. `printf 'payload\n' | xargs -a empty.txt cat` must print
	// `payload`). When the runner exposes RunCommandWithStdin we explicitly
	// pass `emptyChildStdin` only when xargs reads its items from stdin
	// itself (no -a, or `-a -` which is the GNU "stdin" alias); older
	// runners that don't wire RunCommandWithStdin fall back to RunCommand
	// (parent-stdin pass-through).
	xargsReadsStdin := o.argFile == "" || o.argFile == "-"
	var exitCode uint8
	var err error
	if callCtx.RunCommandWithStdin != nil && xargsReadsStdin {
		exitCode, err = callCtx.RunCommandWithStdin(ctx, dir, finalCmd, finalArgs, emptyChildStdin)
	} else {
		exitCode, err = callCtx.RunCommand(ctx, dir, finalCmd, finalArgs)
	}
	if err != nil {
		callCtx.Errf("xargs: %s: %s\n", finalCmd, err.Error())
		// POSIX/GNU xargs distinguishes 127 (command not found) from 125
		// (other fatal startup failure). The runner returns (127, err) for
		// both "command not allowed" and "unknown command"; pass that
		// through unchanged so callers can branch on the canonical code.
		if exitCode == 127 {
			return 127, true
		}
		return exitSubCmdNotStart, true
	}
	switch exitCode {
	case 0:
		return exitOK, false
	case subCmdFatalCode:
		callCtx.Errf("xargs: %s: exited with status 255; aborting\n", finalCmd)
		return exitSubCmd255, true
	default:
		return exitSubCmdFailed, false
	}
}

// resolveCmd assembles the (cmdName, args) pair for a batch, applying -I
// substitution when active.
func resolveCmd(o options, batch []string) (string, []string) {
	if o.useReplace() {
		// -I forces one item per invocation; substitute the single item
		// into every occurrence of replStr in cmdName + initial args.
		var item string
		if len(batch) > 0 {
			item = batch[0]
		}
		cmd := strings.ReplaceAll(o.cmdName, o.replStr, item)
		args := make([]string, len(o.initialArgs))
		for i, a := range o.initialArgs {
			args[i] = strings.ReplaceAll(a, o.replStr, item)
		}
		return cmd, args
	}
	args := make([]string, 0, len(o.initialArgs)+len(batch))
	args = append(args, o.initialArgs...)
	args = append(args, batch...)
	return o.cmdName, args
}

// printVerbose mirrors GNU xargs -t: writes the command line followed by a
// newline to stderr before running it. Each argv element is shell-quoted so
// the printed line is unambiguous when items contain whitespace or other
// shell metacharacters (matches GNU's behavior of tracing
// `echo 'a b'` rather than `echo a b`).
func printVerbose(callCtx *builtins.CallContext, name string, args []string) {
	var b strings.Builder
	b.WriteString(shellQuote(name))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(a))
	}
	b.WriteByte('\n')
	callCtx.Errf("%s", b.String())
}

// shellQuote returns s suitable for printing as a shell-readable argument.
// Empty strings render as `”`; strings without any shell-special characters
// pass through untouched; everything else is wrapped in single quotes with
// internal single quotes escaped via the standard `'\”` sequence.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !containsShellMeta(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b.WriteString(`'\''`)
		} else {
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// containsShellMeta reports whether s contains any byte that would change
// meaning under typical shell parsing if rendered unquoted.
func containsShellMeta(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ', c == '\t', c == '\n', c == '\r', c == '\v', c == '\f':
		case c == '\'', c == '"', c == '\\':
		case c == '$', c == '`':
		case c == '|', c == '&', c == ';':
		case c == '<', c == '>':
		case c == '(', c == ')', c == '{', c == '}':
		case c == '[', c == ']':
		case c == '*', c == '?', c == '~':
		case c == '#', c == '!':
		default:
			continue
		}
		return true
	}
	return false
}

// openInput opens the configured input source. Returns (nil, nil) if no
// stdin is available and no -a file was specified.
//
// `-a -` (or `--arg-file=-`) is the GNU convention for "read items from
// stdin" — it shares the parent's stdin rather than opening a file named
// `-` through the sandbox.
func openInput(ctx context.Context, callCtx *builtins.CallContext, o options) (io.ReadCloser, error) {
	if o.argFile != "" && o.argFile != "-" {
		f, err := callCtx.OpenFile(ctx, o.argFile, os.O_RDONLY, 0)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	if callCtx.Stdin == nil {
		return nil, nil
	}
	return io.NopCloser(callCtx.Stdin), nil
}

// tokenizer reads items from r according to opts.mode, honouring quoting
// and backslash escapes only in modeWhitespace.
type tokenizer struct {
	r         *bufio.Reader
	o         options
	buf       []byte
	eof       bool
	bytesSeen int // running byte count for periodic ctx.Err() polling
	// atLineStart is true when the next token will start at a "line
	// boundary" — either start-of-input or immediately after a '\n'
	// separator. GNU xargs only recognises -E EOF-STR at EOF when the
	// matching token starts at such a boundary; any whitespace
	// terminator (space/tab/newline) recognises it unconditionally.
	atLineStart bool
	// nulSeen is set when a NUL byte was encountered in modeWhitespace
	// or modeLine. The xargs caller polls this after each next() call
	// and emits the GNU-style "NUL character occurred in the input"
	// warning to stderr exactly once per invocation.
	nulSeen bool
	// nulWarned tracks whether the caller has already emitted the
	// stderr warning for this run, so subsequent NUL sightings do not
	// re-warn.
	nulWarned bool
}

func newTokenizer(r io.Reader, o options) *tokenizer {
	return &tokenizer{
		r:           bufio.NewReaderSize(r, readChunk),
		o:           o,
		buf:         make([]byte, 0, 256),
		atLineStart: true,
	}
}

// pollCtx checks ctx cancellation roughly every ctxCheckEvery bytes read.
// Cheap enough to call from the inner read loops without measurable
// throughput impact.
func (t *tokenizer) pollCtx(ctx context.Context) error {
	t.bytesSeen++
	if t.bytesSeen%ctxCheckEvery == 0 {
		return ctx.Err()
	}
	return nil
}

// pushByte appends b to t.buf, returning a "token too long" error when the
// buffer is already at the per-token cap. Centralises the cap check that
// would otherwise be duplicated at every byte append site.
func (t *tokenizer) pushByte(b byte) error {
	if len(t.buf) >= MaxTokenBytes {
		return fmt.Errorf("argument exceeds %d byte limit", MaxTokenBytes)
	}
	t.buf = append(t.buf, b)
	return nil
}

// next returns the next item, an "ended a line" flag (true if the item ends
// with or sits on a line that was terminated by '\n'), a "more" flag (false
// at EOF before producing any item), and any error.
func (t *tokenizer) next(ctx context.Context) (item string, endedLine, more bool, err error) {
	if t.eof {
		return "", false, false, nil
	}
	switch t.o.mode {
	case modeNull:
		return t.nextDelimited(ctx, 0)
	case modeDelim:
		return t.nextDelimited(ctx, t.o.delim)
	case modeLine:
		return t.nextLine(ctx)
	default:
		return t.nextWhitespace(ctx)
	}
}

// nextDelimited reads bytes until the next occurrence of sep or EOF.
// Used by -0 (sep == 0) and -d (sep == o.delim). Quotes and backslashes
// are NOT processed — every byte except sep is taken literally. Each
// delimiter-terminated record is treated as ending a line so -L
// correctly counts batches under `xargs -d , -L1`.
//
// In -d mode (sep != 0), an embedded NUL byte truncates the current
// token at the NUL — GNU xargs lets the bytes flow into argv but real
// `execve` argv is C-string-terminated, so the user-visible token is
// always the prefix up to the first NUL. We mirror that behavior
// silently (no warning) so rshell's Go-string argv matches GNU's
// effective output. -0 mode is unaffected because NUL is the separator.
func (t *tokenizer) nextDelimited(ctx context.Context, sep byte) (string, bool, bool, error) {
	t.buf = t.buf[:0]
	truncated := false
	sawByte := false
	for {
		if err := t.pollCtx(ctx); err != nil {
			return "", false, false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				if !sawByte {
					return "", false, false, nil
				}
				return string(t.buf), true, true, nil
			}
			return "", false, false, err
		}
		sawByte = true
		if b == sep {
			return string(t.buf), true, true, nil
		}
		if b == 0 {
			// sep != 0 here (sep == 0 took the branch above).
			truncated = true
			continue
		}
		if truncated {
			continue
		}
		if err := t.pushByte(b); err != nil {
			return "", false, false, err
		}
	}
}

// nextLine handles -I tokenisation: records are newline-separated, but
// quotes and backslash escapes are still processed within each line
// (matching GNU xargs semantics). Leading whitespace on each record is
// stripped; trailing/internal whitespace is preserved. A backslash
// outside quotes escapes the next byte (including newlines, which joins
// the current record with the following line). A literal '\n' inside an
// open quote is treated as an unterminated quote, matching GNU.
func (t *tokenizer) nextLine(ctx context.Context) (string, bool, bool, error) {
	t.buf = t.buf[:0]

	// Skip leading whitespace, including blank lines.
	for {
		if err := t.pollCtx(ctx); err != nil {
			return "", false, false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				return "", false, false, nil
			}
			return "", false, false, err
		}
		if b == ' ' || b == '\t' || b == '\n' {
			continue
		}
		if err := t.r.UnreadByte(); err != nil {
			return "", false, false, err
		}
		break
	}

	var quote byte
	for {
		if err := t.pollCtx(ctx); err != nil {
			return "", false, false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				if quote != 0 {
					return "", false, false, fmt.Errorf("unterminated %c-quoted string", quote)
				}
				if len(t.buf) == 0 {
					return "", false, false, nil
				}
				if isEofMarker(t.o, t.buf) {
					t.eof = true
					return "", false, false, nil
				}
				return string(t.buf), true, true, nil
			}
			return "", false, false, err
		}

		if quote != 0 {
			if b == quote {
				quote = 0
				continue
			}
			if b == '\n' {
				return "", false, false, fmt.Errorf("unterminated %c-quoted string", quote)
			}
			if err := t.pushByte(b); err != nil {
				return "", false, false, err
			}
			continue
		}

		switch b {
		case '\'', '"':
			quote = b
			continue
		case '\\':
			n, errEsc := t.r.ReadByte()
			if errEsc != nil {
				if errors.Is(errEsc, io.EOF) {
					// GNU silently consumes a trailing backslash at EOF.
					continue
				}
				return "", false, false, errEsc
			}
			// Preserve the escaped byte literally (including newline,
			// which joins the current record with the next line).
			if err := t.pushByte(n); err != nil {
				return "", false, false, err
			}
			continue
		case '\n':
			if isEofMarker(t.o, t.buf) {
				t.eof = true
				return "", false, false, nil
			}
			return string(t.buf), true, true, nil
		case 0:
			// In -I (line) mode, NUL truncates the entire record:
			// emit the current token (possibly empty), warn once, and
			// drop the rest of the line. Subsequent NUL-free lines are
			// tokenised normally. Empty tokens are emitted (matches
			// GNU; e.g. `\0bad\ngood\n` produces `[]\n[good]`).
			t.nulSeen = true
			if err := t.skipToNewline(ctx); err != nil {
				return "", false, false, err
			}
			if isEofMarker(t.o, t.buf) {
				t.eof = true
				return "", false, false, nil
			}
			return string(t.buf), true, true, nil
		}

		if err := t.pushByte(b); err != nil {
			return "", false, false, err
		}
	}
}

// skipToWhitespace drains bytes from t.r until the next whitespace byte
// (space, tab, newline, etc.) or EOF, then unreads the whitespace so the
// outer tokenisation loop can use it as a record terminator. Returns
// true if the terminator was '\n'. Used by the default-mode NUL handler
// so a NUL-contaminated word is dropped only up to the next whitespace
// boundary, leaving subsequent words on the same line intact (matches
// GNU xargs).
func (t *tokenizer) skipToWhitespace(ctx context.Context) (sawNewline bool, err error) {
	for {
		if err := t.pollCtx(ctx); err != nil {
			return false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				return false, nil
			}
			return false, err
		}
		if isWhitespace(b) {
			if b == '\n' {
				t.atLineStart = true
				if err := t.r.UnreadByte(); err != nil {
					return false, err
				}
				return true, nil
			}
			if err := t.r.UnreadByte(); err != nil {
				return false, err
			}
			return false, nil
		}
	}
}

// skipToNewline drains bytes from t.r until '\n' or EOF, leaving the
// reader positioned just after the newline. Used by the NUL-handling
// path so a record containing a NUL byte does not leak its tail bytes
// into the next item.
func (t *tokenizer) skipToNewline(ctx context.Context) error {
	for {
		if err := t.pollCtx(ctx); err != nil {
			return err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				return nil
			}
			return err
		}
		if b == '\n' {
			t.atLineStart = true
			return nil
		}
	}
}

// isWhitespace reports whether b is one of the bytes GNU xargs treats as
// an item terminator in default mode: space, tab, or newline. \r, \v,
// and \f are intentionally NOT included — GNU xargs documents the
// separators as "blanks or newlines", and treating other bytes as
// separators corrupts arguments from CRLF files (e.g. `printf
// 'a\r\nb\r\n' | xargs ...` should pass "a\r" and "b\r", not "a"/"b").
func isWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n':
		return true
	}
	return false
}

// nextWhitespace handles POSIX-style tokenisation with single/double quotes
// and backslash escapes. Blank lines (consecutive whitespace, including
// repeated newlines) are skipped silently and do not contribute to the
// "endedLine" line counter — only lines that actually carry an item count.
func (t *tokenizer) nextWhitespace(ctx context.Context) (string, bool, bool, error) {
	t.buf = t.buf[:0]

	// Skip leading whitespace (including blank lines). Track whether we
	// cross a newline so the next token's "atLineStart" reflects it.
	for {
		if err := t.pollCtx(ctx); err != nil {
			return "", false, false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				return "", false, false, nil
			}
			return "", false, false, err
		}
		if isWhitespace(b) {
			if b == '\n' {
				t.atLineStart = true
			}
			continue
		}
		if err := t.r.UnreadByte(); err != nil {
			return "", false, false, err
		}
		break
	}

	// Snapshot whether this token starts at a line boundary; the EOF
	// path uses it to decide whether a trailing token equal to EOF-STR
	// should be treated as the marker (GNU only recognises it when the
	// token is on its own line — i.e. not wedged onto the same line as
	// earlier content with only a non-newline whitespace separator).
	tokenStartsLine := t.atLineStart

	// quote state: 0 = none, '\'' = single quote, '"' = double quote
	var quote byte
	endedLine := false

	for {
		if err := t.pollCtx(ctx); err != nil {
			return "", false, false, err
		}
		b, err := t.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.eof = true
				if quote != 0 {
					return "", false, false, fmt.Errorf("unterminated %c-quoted string", quote)
				}
				if len(t.buf) == 0 {
					return "", false, false, nil
				}
				// Drop as EOF marker only when the token is on its own
				// line (matches GNU). A trailing token that shares its
				// line with prior content (e.g. `a STOP` with no newline)
				// is treated as a literal item.
				if isEofMarker(t.o, t.buf) && tokenStartsLine {
					t.eof = true
					return "", false, false, nil
				}
				// Token terminated by EOF — count as ending a line.
				return string(t.buf), true, true, nil
			}
			return "", false, false, err
		}

		if quote != 0 {
			if b == quote {
				quote = 0
				continue
			}
			// GNU xargs treats a newline inside a quoted item as an
			// unterminated quote; the input is malformed.
			if b == '\n' {
				return "", false, false, fmt.Errorf("unterminated %c-quoted string", quote)
			}
			if err := t.pushByte(b); err != nil {
				return "", false, false, err
			}
			continue
		}

		switch b {
		case '\'', '"':
			quote = b
			continue
		case '\\':
			n, errEsc := t.r.ReadByte()
			if errEsc != nil {
				if errors.Is(errEsc, io.EOF) {
					// GNU xargs silently consumes a trailing backslash at EOF
					// rather than treating it as an error.
					continue
				}
				return "", false, false, errEsc
			}
			if err := t.pushByte(n); err != nil {
				return "", false, false, err
			}
			continue
		case 0:
			// GNU xargs rejects NUL in non-NUL/-d modes: emit the
			// current token (possibly empty) for the NUL-contaminated
			// word, set the warn flag (caller emits a stderr warning
			// once), and skip remaining bytes up to the next
			// whitespace boundary so the rest of the line (other
			// whitespace-separated words) keeps being tokenised.
			t.nulSeen = true
			sawNewline, err := t.skipToWhitespace(ctx)
			if err != nil {
				return "", false, false, err
			}
			if sawNewline {
				endedLine = true
			}
			if isEofMarker(t.o, t.buf) {
				t.eof = true
				return "", false, false, nil
			}
			return string(t.buf), endedLine, true, nil
		}

		if isWhitespace(b) {
			if b == '\n' {
				endedLine = true
				t.atLineStart = true
			} else {
				t.atLineStart = false
			}
			if isEofMarker(t.o, t.buf) {
				t.eof = true
				return "", false, false, nil
			}
			return string(t.buf), endedLine, true, nil
		}

		// Consuming a non-whitespace byte: the next token (after this
		// one's terminator) is no longer at a line boundary unless the
		// terminator is '\n' (handled above).
		t.atLineStart = false
		if err := t.pushByte(b); err != nil {
			return "", false, false, err
		}
	}
}

// isEofMarker reports whether buf equals the configured EOF-STR. Returns
// false when -E was not set or was cleared (e.g. -0/-d disable -E).
func isEofMarker(o options, buf []byte) bool {
	if o.eofStr == "" {
		return false
	}
	return string(buf) == o.eofStr
}
