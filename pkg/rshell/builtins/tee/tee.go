// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tee implements the tee builtin command.
//
// tee — read from standard input and write to standard output and files
//
// Usage: tee [OPTION]... [FILE]...
//
// Copy standard input to standard output, making a copy in each FILE
// operand. A FILE operand of "-" is opened as a literal file named "-"
// through the sandbox like any other operand — GNU tee treated "-" as an
// alias for stdout in coreutils 5.3.0 through 8.23, but that behavior was
// removed in 8.24 as mandated by POSIX, and this implementation matches
// current GNU tee rather than the historical behavior.
//
// tee mutates file content, so — like truncate, logrotate, rm, and
// systemctl — it is only available in remediation mode. This is enforced
// both at interpreter dispatch (RemediationOnly) and again inside the
// handler as defence in depth, per docs/RULES.md.
//
// Accepted flags:
//
//	-a, --append
//	    Append to the given FILEs, rather than overwriting them.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Out of scope (not implemented; rejected as unknown flags):
//
//	-i, --ignore-interrupts   ignore SIGINT — rshell builtins do not
//	                          receive process signals the way a forked
//	                          tee(1) would, so this flag has no
//	                          meaningful effect to implement.
//	-p, --output-error[=MODE] behavior on write error / FIFO-aware
//	                          diagnostics — every FIFO write target is
//	                          already rejected before open (see "File
//	                          access" below), so the "pipe error" modes
//	                          GNU tee -p exists for cannot occur here.
//
// File access:
//
//	Every FILE operand is opened via callCtx.OpenFile with O_WRONLY |
//	O_CREATE, plus O_APPEND (-a) or O_TRUNC (default). This routes through
//	the same AllowedPaths sandbox and remediation-mode gate that backs the
//	shell's own >/>> redirects and the truncate builtin: writes are
//	confined to :rw roots and a multiply linked (hard-linked) regular file
//	is rejected as a write target. A non-regular target (FIFO, socket,
//	device) is rejected by a non-blocking pre-open StatFile check
//	(rejectNonRegularTarget) so the shell does not block waiting for a
//	FIFO reader; see that function's doc comment for the narrow TOCTOU
//	window this leaves and why it is an accepted, low-severity gap shared
//	with the interpreter's own >/>> redirects. See the hard link entry in
//	AGENTS.md/docs/RULES.md for the shared hard-link mechanism.
//
// Exit codes:
//
//	0  Standard input was fully copied to standard output and to every
//	   FILE operand without error.
//	1  Standard input could not be fully read, or at least one FILE
//	   operand could not be opened or written. tee continues copying to
//	   the remaining destinations after a single non-pipe destination
//	   fails (open/write/close error), matching GNU tee's default
//	   "diagnose errors writing to non pipe outputs" behavior.
//
//	   A broken pipe (EPIPE) on any destination — in practice this is
//	   standard output when a downstream reader like `head` exits early
//	   — is handled differently: matching GNU tee's other documented
//	   default, "exit immediately on error writing to a pipe", the whole
//	   copy stops there rather than continuing to write to the remaining
//	   destinations. This case exits 0, matching cat's handling of the
//	   same condition: a downstream reader exiting early is normal,
//	   expected pipeline behavior, not a reportable error.
//
// Memory safety:
//
//	Input is streamed from stdin in fixed-size chunks (teeBufSize, 32
//	KiB) and fanned out to standard output and every open FILE in the same
//	iteration — no buffering proportional to total input size. The read
//	loop checks ctx.Err() before every Read to honour the shell's
//	execution timeout and support graceful cancellation, and also returns
//	as soon as every destination (including standard output) has stopped
//	accepting writes, rather than draining a long-lived or infinite stdin
//	source to no purpose. The number of concurrently open file descriptors
//	is bounded by MaxFileOperands, checked before any FILE is opened, so a
//	glob or repeated operand cannot exhaust the embedding process's
//	descriptor table; a later duplicate name reuses its own file handle
//	rather than reopening (matching GNU tee's write-order and open-once
//	semantics).
package tee

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/flagparser"
)

// Cmd is the tee builtin command descriptor.
var Cmd = builtins.Command{
	Name:            "tee",
	Description:     "read from standard input and write to standard output and files",
	MakeFlags:       registerFlags,
	RemediationOnly: true,
	// Preserve the historical read-only refusal wording; the dispatch gate
	// in interp emits this before flag parsing, and the in-handler check
	// repeats it as defence in depth.
	RemediationDeniedMessage: readOnlyMessage,
}

const readOnlyMessage = "tee: filesystem capability not available (remediation mode required)\n"
const noWritableRootHint = "tee: no writable path is configured (remediation mode requires an AllowedPaths entry with :rw)\n"

// hasWritableRoot reports whether the sandbox has at least one AllowedPaths
// root configured with :rw access. callCtx.OpenFile/StatFile being non-nil
// only means a sandbox exists, not that it grants any writable root, since
// AllowedPaths wires the sandbox even for an empty list or read-only-only
// entries. Matches truncate's hasWritableRoot exactly.
func hasWritableRoot(callCtx *builtins.CallContext) bool {
	if callCtx.AllowedPathsList == nil {
		return false
	}
	for _, p := range callCtx.AllowedPathsList() {
		if p.Access == builtins.AllowedPathReadWrite {
			return true
		}
	}
	return false
}

// teeBufSize is the fixed-size chunk used to stream stdin to stdout and
// every FILE destination. Bounded and independent of input size.
const teeBufSize = 32 * 1024

// MaxFileOperands is the maximum number of FILE operands accepted by a
// single tee invocation. Every accepted operand is opened and held open
// concurrently for the whole duration of the copy (see the package doc
// comment) — unlike jq/sha256sum, which open, process, and close one file
// at a time, tee cannot release a descriptor until every destination has
// been written to and closed. An unbounded operand count — easily reached
// through shell glob expansion — would therefore open and hold an
// unbounded number of file descriptors, exhausting the embedding process's
// descriptor table (starving unrelated concurrent work in that process)
// well before any generous-looking cap like 1,024 is reached: macOS's
// default soft RLIMIT_NOFILE is 256, and a cap needs headroom below the
// lowest common default for stdin/stdout/stderr and any sandbox-internal
// descriptors already open in the embedding process. 64 matches jq's
// existing MaxOperands precedent for simultaneously-relevant file
// operands, with a wide margin below 256. Exceeding the limit rejects the
// entire command before any destination is opened.
const MaxFileOperands = 64

// dest groups a destination's writer with the name used in diagnostics and
// an optional Closer (nil for standard output, which the handler never
// closes).
type dest struct {
	name   string
	w      io.Writer
	closer io.Closer
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// Both flags use RegisterNoArgBool rather than fs.BoolP so that an
	// explicit value (tee --append=false file, tee --help=false) is
	// rejected with GNU's "doesn't allow an argument" error instead of
	// silently parsing — a bare fs.BoolP flag accepts --flag=value, and
	// --append=false would silently select truncation (capable of
	// overwriting an existing file) instead of being refused, matching
	// GNU tee's own -a/--append and -h/--help, which take no argument.
	help := flagparser.RegisterNoArgBool(fs, "help", "h", "print usage and exit")
	appendFlag := flagparser.RegisterNoArgBool(fs, "append", "a", "append to the given files, do not overwrite")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		// Capability check before everything else — including --help — so
		// that tee --help behaves the same as invoking a disallowed command:
		// it fails immediately without showing help text. This mirrors
		// rm/truncate/logrotate's ordering; the RemediationOnly dispatch gate
		// in interp already enforces this ahead of the handler for ordinary
		// invocations, but the handler itself must also refuse every
		// invocation — including --help — in case the exported command
		// factory is invoked directly rather than through interpreter
		// dispatch (defence in depth, per docs/RULES.md).
		if !callCtx.RemediationMode {
			callCtx.Errf("%s", readOnlyMessage)
			return builtins.Result{Code: 1}
		}

		// callCtx.OpenFile and callCtx.StatFile are wired unconditionally by
		// normal interpreter dispatch, but RemediationMode alone does not
		// guarantee they are set: a caller invoking the exported command
		// factory directly (bypassing interp) could set RemediationMode
		// without wiring either closure, which would otherwise panic on a
		// nil call inside rejectNonRegularTarget/OpenFile below instead of
		// reporting a missing capability. Checking AllowedPathsList
		// directly (rather than only nil-checking the closures) also
		// distinguishes "remediation mode is on but no writable root
		// exists" from a genuinely missing capability, matching
		// truncate's hasWritableRoot guidance so the operator is not sent
		// to fix the wrong thing.
		if callCtx.OpenFile == nil || callCtx.StatFile == nil || !hasWritableRoot(callCtx) {
			callCtx.Errf("%s", noWritableRootHint)
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: tee [OPTION]... [FILE]...\n")
			callCtx.Out("Copy standard input to standard output, making a copy in each FILE.\n\n")

			// RegisterNoArgBool uses an unforgeable NUL sentinel for bare
			// flags. Clear it while rendering defaults so help output
			// contains no NUL byte.
			var saved []*builtins.Flag
			fs.VisitAll(func(flag *builtins.Flag) {
				if flag.NoOptDefVal == flagparser.NoArgSentinel {
					saved = append(saved, flag)
					flag.NoOptDefVal = ""
				}
			})
			defer func() {
				for _, flag := range saved {
					flag.NoOptDefVal = flagparser.NoArgSentinel
				}
			}()

			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		if len(files) > MaxFileOperands {
			callCtx.Errf("tee: too many operands (maximum %d)\n", MaxFileOperands)
			return builtins.Result{Code: 1}
		}

		flags := os.O_WRONLY | os.O_CREATE
		if *appendFlag {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}

		dests := []dest{{name: "standard output", w: callCtx.Stdout}}
		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				return builtins.Result{Code: 1}
			}

			// Escape once and reuse for every diagnostic involving this
			// operand (open failure below, plus any later write/close
			// failure via dest.name in copyToAll/closeAllDests): a FILE
			// operand containing a newline, ESC sequence, or other control
			// character must not be written to stderr raw, or it could
			// forge additional diagnostic lines or inject terminal/log
			// control sequences.
			safeName := builtins.SafeOperand(file)

			// "-" is a literal filename, not a stdout alias: GNU tee
			// dropped that historical special case in coreutils 8.24 (see
			// the package doc comment above). It goes through the same
			// sandbox path as any other operand.
			if err := rejectNonRegularTarget(ctx, callCtx, file); err != nil {
				callCtx.Errf("tee: '%s': %s\n", safeName, safeErr(callCtx, err))
				failed = true
				continue
			}
			f, err := callCtx.OpenFile(ctx, file, flags, 0666)
			if err != nil {
				callCtx.Errf("tee: '%s': %s\n", safeName, safeErr(callCtx, err))
				failed = true
				continue
			}
			// f must stay open for the whole copy below, which happens
			// after every destination has been opened; defer here (rather
			// than closing immediately) keeps it open until the handler
			// returns, which is exactly the lifetime tee needs. It is a
			// safety net only: the explicit close loop below runs first on
			// every non-panicking path and is what actually surfaces a
			// close failure; closing an already-closed file here is a
			// harmless no-op error that is discarded.
			defer f.Close()
			dests = append(dests, dest{name: safeName, w: f, closer: f})
		}

		if callCtx.Stdin != nil {
			if err := copyToAll(ctx, callCtx, callCtx.Stdin, dests); err != nil {
				failed = true
			}
		}

		// Close every FILE destination explicitly (rather than relying only
		// on the defer above) so a write-back failure surfaced only at
		// Close time — e.g. a delayed ENOSPC/EIO/quota error on some
		// filesystems, including several network filesystems — is diagnosed
		// and turns into exit 1 instead of being silently discarded.
		if !closeAllDests(callCtx, dests) {
			failed = true
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// rejectNonRegularTarget prevents opening a non-regular file (FIFO, socket,
// device) as a write destination. Unlike Sandbox.Truncate, callCtx.OpenFile
// does not set O_NONBLOCK on its own write-open path, so an O_WRONLY open of
// a FIFO with no reader would block the shell indefinitely waiting for a
// connection. callCtx.StatFile is openat-based and never blocks, matching
// the interpreter's own rejectNonRegularRedirectTarget guard used for the >
// and >> redirects.
//
// ENOENT is ignored: O_CREATE will create a regular file for a missing
// target, and any other open failure surfaces from the subsequent OpenFile
// call.
//
// This check has a real, if narrow, TOCTOU window: if the target is
// replaced with a FIFO by a concurrent process between this Stat and the
// subsequent OpenFile call, the open can still block — the sandbox's own
// write-open path (allowedpaths.checkWriteTargetLinks) only rejects a
// *regular* file with an excess link count; it does not reject a
// non-regular descriptor post-open the way this comment previously (and
// incorrectly) claimed. Closing this window completely would require a
// sandbox primitive that opens O_NONBLOCK and validates the resulting
// descriptor atomically, the way Sandbox.Truncate already does for its own
// call site; no such primitive is exposed through callCtx today, so this
// pre-open stat is the best available mitigation at the builtin layer. The
// interpreter's own `>`/`>>` redirects have an identical window through the
// same rejectNonRegularRedirectTarget-then-Open sequence, so this is not a
// new risk introduced by tee; it is an accepted, low-severity gap (it needs
// a concurrent writer with access to the same :rw root, racing a narrow
// window, to turn a normal write into a hang rather than a sandbox escape
// — path containment itself is enforced atomically via openat regardless).
func rejectNonRegularTarget(ctx context.Context, callCtx *builtins.CallContext, path string) error {
	info, err := callCtx.StatFile(ctx, path)
	if err != nil {
		return nil // ENOENT or other: let OpenFile surface the real error
	}
	if info.Mode()&iofs.ModeType == 0 {
		return nil // regular file
	}
	return &os.PathError{Op: "open", Path: path, Err: errNotRegularFile}
}

// errNotRegularFile is the sentinel wrapped by rejectNonRegularTarget's
// PathError. Its text matches the wording callCtx.PortableErr already
// produces for the sandbox's own post-open regular-file rejection (see
// writeopen.ErrNotRegularFile), so the user-visible message is identical
// whether tee is stopped by this pre-open stat check or — in the rarer case
// where a FIFO is swapped in between — by the sandbox's own guard.
var errNotRegularFile = errors.New("not a regular file")

// copyToAll streams src to every live destination in dests in fixed-size
// chunks, removing a destination from the active set the first time it
// returns a write error (matching GNU tee: a failing destination is
// reported once and dropped, while output continues to the rest). It
// returns a non-nil error if reading src failed or if any destination
// write failed at any point.
func copyToAll(ctx context.Context, callCtx *builtins.CallContext, src io.Reader, dests []dest) error {
	buf := make([]byte, teeBufSize)
	live := make([]bool, len(dests))
	liveCount := len(dests)
	for i := range live {
		live[i] = true
	}
	anyFailed := false

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			brokenPipe := false
			for i, d := range dests {
				if !live[i] {
					continue
				}
				if _, werr := d.w.Write(chunk); werr != nil {
					if builtins.IsBrokenPipe(werr) {
						// GNU tee's documented default ("exit immediately
						// on error writing to a pipe") applies regardless of
						// which destination broke — upstream, real tee lets
						// SIGPIPE terminate the whole process on the first
						// EPIPE, whether that write targets stdout or
						// another pipe-like destination; it does not keep
						// writing to remaining destinations. -p/
						// --output-error, which changes that to "warn and
						// keep going", is out of scope here (see the
						// package doc comment). Matching cat's handling of
						// the same condition, this is a silent, successful
						// stop — not a reported failure — since a reader
						// exiting early is normal, expected pipeline
						// behavior.
						brokenPipe = true
						break
					}
					callCtx.Errf("tee: %s: %s\n", d.name, safeErr(callCtx, werr))
					live[i] = false
					liveCount--
					anyFailed = true
				}
			}
			if brokenPipe {
				break
			}
			// Once every destination — including standard output — has
			// stopped accepting writes, there is nothing left to do with
			// further input. Returning here (rather than continuing to
			// drain src to EOF) prevents a long-lived or infinite stdin
			// source from being read forever after the last live
			// destination closes, e.g. `tee | head` once head has exited.
			if liveCount == 0 {
				break
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			callCtx.Errf("tee: read error: %s\n", safeErr(callCtx, readErr))
			anyFailed = true
			break
		}
	}

	if anyFailed {
		return errCopyFailed
	}
	return nil
}

// safeErr formats err via callCtx.PortableErr and escapes any embedded
// newline before it reaches stderr.
//
// PortableErrMsg normally maps common errors (ENOENT, EACCES, etc.) to a
// fixed string with no path in it, so this is usually a no-op. But when an
// error has already been run through allowedpaths.PortablePathError once
// (as Sandbox.Open's write-open path does internally), a second
// PortableErr call here can no longer match the now-generic wrapped error
// against fs.ErrNotExist/etc., and falls back to the raw *os.PathError's
// Error() string — which embeds the operand's Path a second time,
// unescaped. That double-normalization gap is pre-existing in the shared
// allowedpaths layer (reproducible identically through the interpreter's
// own `>`/`>>` redirects, which hit the exact same code path), not
// something specific to tee.
//
// builtins.SafeMessage (not builtins.SafeOperand) is used here: SafeOperand
// also escapes literal backslashes, which is correct for a raw
// user-supplied operand but wrong for an already-formatted message that
// legitimately contains a Windows path separator ("openat
// missing_dir\out.txt: ...") — escaping those would double every
// backslash. SafeMessage escapes the same control-character set (newline,
// CR, tab, ESC, and other non-printable/Unicode line-or-format runes) but
// leaves backslash untouched.
func safeErr(callCtx *builtins.CallContext, err error) string {
	return builtins.SafeMessage(callCtx.PortableErr(err))
}

// closeAllDests closes every destination's closer (skipping the nil closer
// on standard output), reporting each failure to stderr and continuing so
// one bad close does not leave the rest of the descriptors open longer than
// necessary. It returns false if any close failed.
func closeAllDests(callCtx *builtins.CallContext, dests []dest) bool {
	ok := true
	for _, d := range dests {
		if d.closer == nil {
			continue
		}
		if err := d.closer.Close(); err != nil {
			callCtx.Errf("tee: %s: %s\n", d.name, safeErr(callCtx, err))
			ok = false
		}
	}
	return ok
}

// errCopyFailed is a sentinel returned by copyToAll to signal that at least
// one destination write or the stdin read failed. Its text is never printed;
// the caller only checks err != nil, since each failure was already reported
// to stderr at the point it occurred (matching GNU tee's per-destination
// diagnostics instead of a single generic message).
var errCopyFailed = errors.New("tee: one or more destinations failed")
