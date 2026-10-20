// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func (r *Runner) fillExpandConfig(ctx context.Context) {
	r.ectx = ctx
	r.ecfg = &expand.Config{
		Env:      expandEnv{r},
		CmdSubst: r.cmdSubst,
	}
	r.updateExpandOpts()
}

func (r *Runner) updateExpandOpts() {
	r.ecfg.ReadDir2 = func(s string) ([]fs.DirEntry, error) {
		if r.globReadDirCount != nil {
			if r.globReadDirCount.Add(1) > MaxGlobReadDirCalls {
				return nil, fmt.Errorf("glob expansion exceeded maximum number of directory reads (%d)", MaxGlobReadDirCalls)
			}
		}
		ctx := r.handlerCtx(r.ectx, todoPos)
		if r.readDirHandler != nil {
			return r.readDirHandler(ctx, s)
		}
		// Fallback when a custom openHandler was set without a readDirHandler.
		return r.sandbox.ReadDirForGlob(s, HandlerCtx(ctx).Dir)
	}
}

// maxCmdSubstOutput is the maximum number of bytes a command substitution
// can capture before being truncated. This prevents memory exhaustion from
// commands that produce unbounded output.
const maxCmdSubstOutput = 1 << 20 // 1 MiB

// MaxExpandedArgumentsPerCommand is the maximum number of arguments one
// command may receive after shell expansion. The command name itself does not
// count against this limit. The same value bounds a for-loop's expanded word
// list, where there is no command-name field.
const MaxExpandedArgumentsPerCommand = 16 << 10

// MaxExpandedBytesPerCommand is the maximum combined size of fields produced
// for one command or for-loop word list.
const MaxExpandedBytesPerCommand = 10 << 20 // 10 MiB

// MaxExpandedBytesPerRun is the cumulative expansion budget shared by the
// entire Run invocation, including subshells and pipeline stages.
const MaxExpandedBytesPerRun = 64 << 20 // 64 MiB

// maxStdoutBytes is the maximum number of bytes a script can write to stdout
// before further output is silently discarded. This caps total script output
// to prevent memory exhaustion from runaway commands (e.g. infinite loops
// writing to stdout).
const maxStdoutBytes = 10 * 1024 * 1024 // 10 MiB

// maxStderrBytes is the maximum number of bytes a script can write to stderr
// before further output is silently discarded. Symmetric with maxStdoutBytes:
// without an stderr cap, a script that pipes stdin through `while read line;
// do echo "$line" >&2; done` can exhaust the memory of any consumer that
// buffers stderr (test harnesses, agent SDKs, log shippers).
const maxStderrBytes = 10 * 1024 * 1024 // 10 MiB

// MaxGlobReadDirCalls is the maximum number of ReadDirForGlob invocations
// allowed per Run() call. This prevents memory exhaustion from scripts that
// trigger an excessive number of glob expansions (e.g. millions of unquoted
// * tokens, or deeply nested glob patterns in loops).
const MaxGlobReadDirCalls = 10_000

// cmdSubst handles command substitution ($(...) and `...`).
// It runs the commands in a subshell and writes their stdout to w.
//
// Special case: the POSIX `$(<file)` shortcut reads file contents
// directly without spawning a subshell. Because that path performs a
// file read without invoking any command, it would bypass the
// AllowedCommands allowlist if left unchecked. We therefore gate the
// shortcut on `cat` being an allowed command — `$(<file)` is treated as
// an implicit `$(cat file)` for allowlist purposes.
func (r *Runner) cmdSubst(w io.Writer, cs *syntax.CmdSubst) error {
	if len(cs.Stmts) == 0 {
		return nil
	}

	// $(<file) shortcut: read file contents directly without a subshell.
	if word := catShortcutArg(cs.Stmts[0]); word != nil && len(cs.Stmts) == 1 {
		if !r.allowAllCommands && !r.allowedCommands["cat"] {
			r.errf("$(<file): file read not permitted (cat not in allowed commands)\n")
			r.lastExpandExit = exitStatus{code: 1}
			r.lastExit = r.lastExpandExit
			return nil
		}
		path, err := r.redirectWord(r.ectx, word)
		if err != nil {
			r.lastExpandExit = exitStatus{code: 1}
			r.lastExit = r.lastExpandExit
			return nil
		}
		f, err := r.open(r.ectx, path, os.O_RDONLY, 0, true)
		if err != nil {
			// r.open already printed the error; set exit status and
			// return nil so the expand layer does not double-report.
			r.lastExpandExit = exitStatus{code: 1}
			r.lastExit = r.lastExpandExit
			return nil
		}
		defer f.Close()
		// If the path is a directory, silently produce empty output (matches bash).
		if st, ok := f.(interface{ Stat() (fs.FileInfo, error) }); ok {
			if fi, serr := st.Stat(); serr == nil && fi.IsDir() {
				r.lastExpandExit = exitStatus{code: 0}
				r.lastExit = r.lastExpandExit
				return nil
			}
		}
		_, err = io.Copy(w, io.LimitReader(f, maxCmdSubstOutput))
		var exitCode uint8
		if err != nil {
			exitCode = 1
		}
		r.lastExpandExit = exitStatus{code: exitCode}
		r.lastExit = r.lastExpandExit
		return err
	}

	// General case: run statements in a subshell, capturing stdout.
	var buf bytes.Buffer
	r2 := r.subshell(false)
	r2.stdout = &limitWriter{w: &buf, limit: maxCmdSubstOutput}
	// $(...) inherits the parent's loop context: bash silently no-ops
	// break/continue invoked inside a command substitution rather than
	// printing the "only useful in a loop" diagnostic. Counters do not
	// escape the subshell — only the diagnostic is suppressed.
	r2.inLoop = r.inLoop
	r2.stmts(r.ectx, cs.Stmts)
	r2.exit.exiting = false
	r.lastExpandExit = r2.exit
	r.lastExit = r.lastExpandExit
	if r2.exit.fatalExit {
		// Propagate the fatal state to the parent runner so that
		// callers (e.g. for loops iterating over $(…)) cannot
		// silently continue after a context cancellation or other
		// fatal error in the subshell.
		r.exit.fatal(r2.exit.err)
		return r2.exit.err
	}
	if r2.exit.limitExit {
		r.exit.code = 1
		r.exit.exiting = true
		r.exit.limitExit = true
		return nil
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// catShortcutArg detects the $(<file) pattern: a single statement with no
// command and exactly one input redirection. Returns the redirect word if
// matched, nil otherwise.
func catShortcutArg(stmt *syntax.Stmt) *syntax.Word {
	if stmt.Cmd != nil || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil
	}
	if len(stmt.Redirs) != 1 {
		return nil
	}
	rd := stmt.Redirs[0]
	if rd.Op != syntax.RdrIn {
		return nil
	}
	return rd.Word
}

// limitWriter wraps a writer and stops writing after limit bytes.
// When the limit is exceeded, exceeded is set to true and further writes
// are silently discarded so that callers do not see spurious short-write
// errors mid-execution. The exceeded flag can be checked after execution
// via isExceeded to surface the event as an error.
//
// limitWriter is safe for concurrent use: the mutex serialises writes so
// that the byte counter and exceeded flag are always consistent, even when
// background goroutines write to the same writer concurrently.
type limitWriter struct {
	mu       sync.Mutex
	w        io.Writer
	limit    int64
	n        int64
	exceeded bool
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.n >= lw.limit {
		lw.exceeded = true
		return len(p), nil // silently discard excess
	}
	remaining := lw.limit - lw.n
	if int64(len(p)) > remaining {
		if _, err := lw.w.Write(p[:remaining]); err != nil {
			return int(remaining), err
		}
		lw.n = lw.limit
		lw.exceeded = true
		return len(p), nil // report all bytes consumed to avoid short-write errors
	}
	n, err := lw.w.Write(p)
	lw.n += int64(n)
	return n, err
}

// isExceeded reports whether any write has exceeded the byte limit.
// It is safe to call concurrently with Write.
func (lw *limitWriter) isExceeded() bool {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.exceeded
}

func (r *Runner) expandErr(err error) {
	if err == nil {
		return
	}
	errMsg := err.Error()
	fmt.Fprintln(r.stderr, errMsg)
	var storageErr *errTotalVarStorageExceeded
	var limitErr *expansionLimitError
	switch {
	case errors.As(err, &limitErr):
		// Resource-limit failures abort the script. Continuing would let a loop
		// repeatedly consume the same bounded allocation and CPU budget.
		r.exit.limitExit = true
	case errors.As(err, &expand.UnsetParameterError{}):
	case errors.As(err, &expand.UnexpectedCommandError{}):
		// Defense in depth: if the expand package encounters a command
		// substitution that our handler cannot process, treat it as fatal.
	case errors.As(err, &storageErr):
		// Total variable storage exhaustion via parameter expansion (e.g.
		// ${var:=value}) must abort the script, just as direct assignment
		// through setVar does.  Without this arm the error falls through to
		// the default case, which only sets exit code 1 and lets the script
		// continue — a cap bypass.
	case errMsg == "invalid indirect expansion":
		// TODO: These errors are treated as fatal by bash.
		// Make the error type reflect that.
	case strings.HasSuffix(errMsg, "not supported"):
		// TODO: This "has suffix" is a temporary measure until the expand
		// package supports all syntax nodes like extended globbing.
	default:
		// Non-fatal expansion errors (e.g. assignment to a readonly variable):
		// set non-zero exit status so the failure is visible, but do not exit
		// the script — bash continues execution in this case.
		r.exit.code = 1
		return
	}
	r.exit.code = 1
	r.exit.exiting = true
}

type expansionLimitError struct {
	message string
}

func (e *expansionLimitError) Error() string { return e.message }

// mvdan.cc/sh/v3 does not expose a typed error for its brace expansion cap.
// Keep the text dependency isolated; nested-limit tests pin fatal propagation.
const braceExpansionLimitErrorPrefix = "brace expansion would exceed "

type fieldCollector struct {
	r         *Runner
	maxFields int
	fields    []string
	bytes     int64
}

func (r *Runner) newFieldCollector(maxFields int) *fieldCollector {
	return &fieldCollector{r: r, maxFields: maxFields}
}

func (c *fieldCollector) setCommandPrefixFields(n int) bool {
	c.maxFields = n + MaxExpandedArgumentsPerCommand
	if len(c.fields) > c.maxFields {
		c.r.expandErr(&expansionLimitError{message: fmt.Sprintf(
			"expansion exceeds maximum argument count (%d)", MaxExpandedArgumentsPerCommand)})
		return false
	}
	c.bytes = 0
	for _, field := range c.fields[n:] {
		c.bytes += int64(len(field))
	}
	return true
}

func (c *fieldCollector) add(words ...*syntax.Word) bool {
	for _, word := range words {
		if c.r.stop(c.r.ectx) {
			return false
		}
		remaining := int64(MaxExpandedBytesPerCommand) - c.bytes
		estimatedWord := *word
		syntax.SplitBraces(&estimatedWord)
		if err := c.r.checkWordExpansionSize(&estimatedWord, remaining,
			fmt.Sprintf("expansion exceeds maximum command size (%d bytes)", MaxExpandedBytesPerCommand)); err != nil {
			c.r.expandErr(err)
			return false
		}

		for field, err := range expand.FieldsSeq(c.r.ecfg, word) {
			if err != nil {
				if strings.HasPrefix(err.Error(), braceExpansionLimitErrorPrefix) {
					err = &expansionLimitError{message: err.Error()}
				}
				c.r.expandErr(err)
				return false
			}
			if c.r.stop(c.r.ectx) {
				return false
			}
			if len(c.fields) >= c.maxFields {
				c.r.expandErr(&expansionLimitError{message: fmt.Sprintf(
					"expansion exceeds maximum field count (%d)", c.maxFields)})
				return false
			}
			fieldBytes := int64(len(field))
			if fieldBytes > int64(MaxExpandedBytesPerCommand)-c.bytes {
				c.r.expandErr(&expansionLimitError{message: fmt.Sprintf(
					"expansion exceeds maximum command size (%d bytes)", MaxExpandedBytesPerCommand)})
				return false
			}
			if err := c.r.chargeExpansionBytes(fieldBytes); err != nil {
				c.r.expandErr(err)
				return false
			}
			c.bytes += fieldBytes
			c.fields = append(c.fields, field)
		}
	}
	return true
}

func (r *Runner) fields(words ...*syntax.Word) []string {
	collector := r.newFieldCollector(MaxExpandedArgumentsPerCommand)
	collector.add(words...)
	return collector.fields
}

func (r *Runner) literal(word *syntax.Word) string {
	return r.literalBounded(word, MaxExpandedBytesPerCommand,
		fmt.Sprintf("expansion exceeds maximum size (%d bytes)", MaxExpandedBytesPerCommand))
}

func (r *Runner) assignmentLiteral(name string, word *syntax.Word) string {
	return r.literalBounded(word, MaxVarBytes,
		fmt.Sprintf("%s: value too large (limit %d bytes)", name, MaxVarBytes))
}

func (r *Runner) literalBounded(word *syntax.Word, limit int64, message string) string {
	if err := r.checkWordExpansionSize(word, limit, message); err != nil {
		r.expandErr(err)
		return ""
	}
	str, err := expand.Literal(r.ecfg, word)
	if err == nil && int64(len(str)) > limit {
		err = &expansionLimitError{message: message}
	}
	if err == nil {
		err = r.chargeExpansionBytes(int64(len(str)))
	}
	r.expandErr(err)
	if err != nil {
		return ""
	}
	return str
}

func (r *Runner) document(word *syntax.Word) (string, error) {
	message := fmt.Sprintf("heredoc: content exceeds maximum size (%d bytes)", MaxHeredocBytes)
	if err := r.checkWordExpansionSize(word, MaxHeredocBytes, message); err != nil {
		return "", err
	}
	str, err := expand.Document(r.ecfg, word)
	if err == nil && len(str) > MaxHeredocBytes {
		err = &expansionLimitError{message: message}
	}
	if err != nil {
		return "", err
	}
	return str, nil
}

func (r *Runner) chargeExpansionBytes(n int64) error {
	if n <= 0 || r.expansionByteCount == nil {
		return nil
	}
	total := r.expansionByteCount.Add(n)
	if total > MaxExpandedBytesPerRun {
		return &expansionLimitError{message: fmt.Sprintf(
			"expansion exceeds maximum cumulative size (%d bytes)", MaxExpandedBytesPerRun)}
	}
	return nil
}

func (r *Runner) checkWordExpansionSize(word *syntax.Word, limit int64, message string) error {
	if word == nil {
		return nil
	}
	size, err := r.wordPartsExpansionSize(word.Parts)
	if err != nil {
		return err
	}
	if size.known > limit {
		return &expansionLimitError{message: message}
	}
	if size.maximum > MaxExpandedBytesPerCommand {
		return &expansionLimitError{message: fmt.Sprintf(
			"expansion exceeds maximum intermediate size (%d bytes)", MaxExpandedBytesPerCommand)}
	}
	return nil
}

type wordExpansionSize struct {
	// known is the exact upper bound of parts whose values are already
	// available. maximum also reserves the output cap of command
	// substitutions, whose actual size is unknowable until they run.
	known   int64
	maximum int64
}

func addExpansionSize(a, b int64) int64 {
	const saturated = int64(MaxExpandedBytesPerCommand) + 1
	if a >= saturated || b >= saturated || b > saturated-a {
		return saturated
	}
	return a + b
}

func (r *Runner) wordPartsExpansionSize(parts []syntax.WordPart) (wordExpansionSize, error) {
	var total wordExpansionSize
	for _, part := range parts {
		var size wordExpansionSize
		switch part := part.(type) {
		case *syntax.Lit:
			size.known = int64(len(part.Value))
			size.maximum = size.known
		case *syntax.SglQuoted:
			// ANSI-C quoting can only shrink the source representation: escape
			// sequences are decoded and embedded NUL terminates the value.
			size.known = int64(len(part.Value))
			size.maximum = size.known
		case *syntax.DblQuoted:
			var err error
			size, err = r.wordPartsExpansionSize(part.Parts)
			if err != nil {
				return wordExpansionSize{}, err
			}
		case *syntax.ParamExp:
			if part.Param == nil {
				return wordExpansionSize{}, fmt.Errorf("unsupported parameter expansion")
			}
			size.known = int64(len(r.lookupVar(part.Param.Value).String()))
			size.maximum = size.known
		case *syntax.CmdSubst:
			// The command has not run yet, so its minimum is unknown while its
			// maximum is the existing hard output cap. This lets ordinary forms
			// such as prefix$(cmd) proceed while still bounding the intermediate
			// string before the exact post-expansion check.
			size.maximum = maxCmdSubstOutput
		case *syntax.BraceExp:
			// A brace expansion emits one alternative at a time. Bound the
			// largest possible alternative; FieldsSeq separately caps how many
			// alternatives may be emitted.
			for _, elem := range part.Elems {
				elemSize, err := r.wordPartsExpansionSize(elem.Parts)
				if err != nil {
					return wordExpansionSize{}, err
				}
				if elemSize.known > size.known {
					size.known = elemSize.known
				}
				if elemSize.maximum > size.maximum {
					size.maximum = elemSize.maximum
				}
			}
		default:
			// validateNode rejects every other WordPart. Fail closed for direct
			// internal callers and any AST variants added upstream.
			return wordExpansionSize{}, fmt.Errorf("unsupported expansion part %T", part)
		}
		total.known = addExpansionSize(total.known, size.known)
		total.maximum = addExpansionSize(total.maximum, size.maximum)
	}
	return total, nil
}

// expandEnv exposes [Runner]'s variables to the expand package.
type expandEnv struct {
	r *Runner
}

var _ expand.WriteEnviron = expandEnv{}

func (e expandEnv) Get(name string) expand.Variable {
	return e.r.lookupVar(name)
}

func (e expandEnv) Set(name string, vr expand.Variable) error {
	if err := e.r.setVarErr(name, vr); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (e expandEnv) Each(fn func(name string, vr expand.Variable) bool) {
	e.r.writeEnv.Each(fn)
}
