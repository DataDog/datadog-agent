// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package truncate implements the truncate builtin command.
//
// truncate — shrink or extend the size of a file to a specified size
//
// Usage: truncate [OPTION]... FILE...
//
// Shrink or extend the size of each FILE to the specified size. A file that
// is larger than the specified size is truncated; a file that is smaller is
// extended (the extension reads as zero bytes). When the file does not yet
// exist it is created (mode 0666 & ~umask) unless --no-create is given.
//
// All file operations go through the AllowedPaths sandbox. Targets whose
// *path* falls outside the sandbox are rejected with a permission error
// before any open syscall is issued. Path-based containment cannot see inode
// aliasing, so the sandbox additionally rejects any multiply linked regular
// file (link count above one) as a write target: a pre-existing hard link
// inside a read-write root could otherwise name an inode that is also named
// outside every configured root. See the hard link entry in AGENTS.md. This
// command is only available in remediation mode.
//
// Accepted flags:
//
//	-s SIZE, --size=SIZE
//	    Set the file size to SIZE bytes. SIZE is a non-negative integer
//	    with an optional suffix.
//
//	    For K/M/G/T the leading letter is case-insensitive; for P/E the
//	    leading letter is uppercase-only (matching GNU truncate exactly).
//	    The trailing "B" and "iB" characters are always case-sensitive:
//
//	        K = k = KiB = kiB = 1024         KB = kB = 1000
//	        M = m = MiB = miB = 1024^2       MB = mB = 1000^2
//	        G = g = GiB = giB = 1024^3       GB = gB = 1000^3
//	        T = t = TiB = tiB = 1024^4       TB = tB = 1000^4
//	        P = PiB = 1024^5                 PB = 1000^5
//	        E = EiB = 1024^6                 EB = 1000^6
//
//	    Z/Y/R/Q (zetta/yotta/ronna/quetta) are rejected because their
//	    multipliers exceed int64. GNU coreutils with the standard 64-bit
//	    uintmax_t rejects these as well.
//
//	-c, --no-create
//	    Do not create files that do not already exist. Missing files are
//	    silently skipped (matching GNU truncate).
//
//	-h, --help
//	    Print this usage message to stdout and exit 0.
//
// Out of scope (not implemented; rejected as unknown flags):
//
//	-r REF, --reference=FILE   set size from a reference file
//	-o, --io-blocks            treat SIZE as a block count
//	relative size modifiers in -s (+, -, <, >, /, %)
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one file failed (invalid size, permission denied, missing
//	   file without -c, etc.). Processing continues across all operands so
//	   that a single failure does not abort the run; exit 1 is returned at
//	   the end if any operand failed.
//
// Memory safety:
//
//	truncate performs no I/O on file contents — only metadata. The sandbox
//	opens the file with O_WRONLY (+ O_CREATE when allowed) and calls
//	ftruncate(2) on the resulting fd. No buffers are allocated proportional
//	to user input; the only user-controlled numeric is the size argument,
//	which is validated for overflow before reaching the kernel.
package truncate

import (
	"context"
	"errors"
	"os"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/sizeparse"
)

// Cmd is the truncate builtin command descriptor.
var Cmd = builtins.Command{
	Name:            "truncate",
	Description:     "shrink or extend file size",
	MakeFlags:       registerFlags,
	RemediationOnly: true,
	// Preserve the historical read-only refusal wording; the dispatch gate
	// in interp emits this before flag parsing, and the in-handler check
	// repeats it as defence in depth.
	RemediationDeniedMessage: readOnlyMessage,
}

const (
	readOnlyMessage    = "truncate: filesystem capability not available (remediation mode required)\n"
	noWritableRootHint = "truncate: no writable path is configured (remediation mode requires an AllowedPaths entry with :rw)\n"
)

// errInvalidSize is returned by parseSize for any non-numeric, malformed,
// or out-of-range input.
var errInvalidSize = sizeparse.ErrInvalid

// errRelativeSize is returned by parseSize when the size value carries a
// leading +/-/<>//%/% modifier. We surface a dedicated error so the handler
// can hint that these forms are intentionally not supported.
var errRelativeSize = sizeparse.ErrRelative

// hasWritableRoot reports whether the sandbox has at least one AllowedPaths
// root configured with :rw access. callCtx.Truncate being non-nil only means
// a sandbox exists, not that it grants any writable root, since AllowedPaths
// wires the sandbox even for an empty list or read-only-only entries.
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

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	sizeStr := fs.StringP("size", "s", "", "set file size to SIZE bytes")
	noCreate := fs.BoolP("no-create", "c", false, "do not create files that do not exist")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		// Capability check before everything else — including --help — so that
		// truncate --help behaves the same as invoking a disallowed command:
		// it fails immediately without showing help text.
		//
		// callCtx.Truncate is wired whenever remediation mode is on and any
		// AllowedPaths option was configured at all — even an explicit empty
		// list or read-only-only roots — so a nil check alone cannot detect
		// "remediation mode is on but no writable root exists". Check
		// AllowedPathsList directly so that case gets the same guidance
		// instead of falling through to a per-file "permission denied".
		if !callCtx.RemediationMode {
			callCtx.Errf("%s", readOnlyMessage)
			return builtins.Result{Code: 1}
		}
		if callCtx.Truncate == nil || !hasWritableRoot(callCtx) {
			callCtx.Errf("%s", noWritableRootHint)
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: truncate [OPTION]... FILE...\n")
			callCtx.Out("Shrink or extend the size of each FILE to the specified size.\n")
			callCtx.Out("A FILE smaller than SIZE is extended with zero bytes; a FILE\n")
			callCtx.Out("larger than SIZE is truncated. Missing files are created unless\n")
			callCtx.Out("--no-create is given.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		if !fs.Changed("size") {
			callCtx.Errf("truncate: you must specify --size\n")
			return builtins.Result{Code: 1}
		}

		size, err := parseSize(*sizeStr)
		if err != nil {
			if errors.Is(err, errRelativeSize) {
				callCtx.Errf("truncate: invalid size %q: %s\n", *sizeStr, err)
			} else {
				callCtx.Errf("truncate: invalid size %q\n", *sizeStr)
			}
			return builtins.Result{Code: 1}
		}

		if len(files) == 0 {
			callCtx.Errf("truncate: missing file operand\n")
			return builtins.Result{Code: 1}
		}

		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				return builtins.Result{Code: 1}
			}
			err := callCtx.Truncate(ctx, file, size, !*noCreate)
			if err == nil {
				continue
			}
			if *noCreate && errors.Is(err, os.ErrNotExist) {
				continue
			}
			// The underlying failure can come from open (permission
			// denied, not-a-regular-file, ENOENT without -c) or from
			// ftruncate (ENOSPC, EFBIG, EINVAL); use a phase-neutral
			// message so the operator is not misled when an open
			// succeeded but the size change failed.
			callCtx.Errf("truncate: %q: %s\n", file, callCtx.PortableErr(err))
			failed = true
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// parseSize parses the value of -s/--size into a non-negative byte count.
//
// The grammar matches GNU truncate:
//
//	size := digit+ suffix?
//	suffix := [Kk] | [Kk]B | [Kk]iB |
//	          [Mm] | [Mm]B | [Mm]iB |
//	          [Gg] | [Gg]B | [Gg]iB |
//	          [Tt] | [Tt]B | [Tt]iB |
//	          P    | PB    | PiB    |
//	          E    | EB    | EiB
//
// Leading +/-/<>//% modifiers (the GNU relative-size syntax) are rejected
// with errRelativeSize so the caller can surface a hint. Any other malformed
// input, or any value whose product overflows int64, returns errInvalidSize.
func parseSize(s string) (int64, error) {
	return sizeparse.Parse(s)
}
