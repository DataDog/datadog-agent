// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package logrotate implements a remediation-mode log truncation builtin.
//
// logrotate — truncate logs by size threshold or force
//
// Usage: logrotate (-s SIZE|-f) [OPTION]... FILE...
//
// This is a deliberately small, rshell-safe subset inspired by logrotate(8).
// It truncates each FILE to zero bytes through the AllowedPaths sandbox. It
// does not parse logrotate config files, rename logs, retain rotated copies,
// compress output, write state files, run pre/post-rotate scripts, or follow
// symlinked write targets.
//
// Accepted flags:
//
//	-s SIZE, --size=SIZE
//	    Only truncate files whose current size is at least SIZE bytes. SIZE
//	    uses the same non-negative coreutils suffix grammar as truncate -s.
//	    -s 0 is accepted and is intentionally equivalent to --force: every
//	    non-empty file clears a zero threshold. Empty files are never
//	    rewritten either way, so the two spellings cannot differ.
//
//	-f, --force
//	    Truncate without a size threshold. Mutually exclusive with --size.
//
//	-n, --dry-run
//	    Print what would happen without modifying files. Dry-run implies
//	    per-file reporting even when --verbose is not set.
//
//	-v, --verbose
//	    Print a line per file describing whether it was truncated or skipped.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  Bad flag value, missing operand, or at least one per-file failure.
//	   Processing continues across operands so a single failure does not
//	   abort the run.
package logrotate

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/flagparser"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/sizeparse"
)

// Cmd is the logrotate builtin command descriptor.
var Cmd = builtins.Command{
	Name:            "logrotate",
	Description:     "truncate logs by size threshold or force",
	MakeFlags:       registerFlags,
	RemediationOnly: true,
	// Preserve the historical read-only refusal wording; the dispatch gate
	// in interp emits this before flag parsing, and the in-handler check
	// repeats it as defence in depth.
	RemediationDeniedMessage: readOnlyMessage,
}

const (
	readOnlyMessage    = "logrotate: filesystem capability not available (remediation mode required)\n"
	noWritableRootHint = "logrotate: no writable path is configured (remediation mode requires an AllowedPaths entry with :rw)\n"
)

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// The boolean flags use RegisterNoArgBool rather than fs.BoolP so that an
	// explicit value (--force=false, --dry-run=false) is rejected with GNU's
	// "doesn't allow an argument" error instead of silently parsing, matching
	// rm. This matters most for --dry-run: a bare fs.BoolP flag accepts
	// `--dry-run=false`, so a malformed no-argument option would silently turn
	// a preview into a real truncation.
	help := flagparser.RegisterNoArgBool(fs, "help", "h", "print usage and exit")
	sizeStr := fs.StringP("size", "s", "", "only truncate files at least SIZE bytes")
	force := flagparser.RegisterNoArgBool(fs, "force", "f", "truncate without a size threshold")
	dryRun := flagparser.RegisterNoArgBool(fs, "dry-run", "n", "show what would be truncated without modifying files")
	verbose := flagparser.RegisterNoArgBool(fs, "verbose", "v", "print each truncated or skipped file")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		// Capability check before everything else — including --help — so that
		// logrotate --help behaves like invoking any remediation-only builtin
		// outside remediation mode.
		if callCtx.TruncateToZeroIfAtLeast == nil {
			// Remediation mode with no sandbox root is a configuration gap,
			// not a mode problem — say so rather than blaming the mode.
			if callCtx.RemediationMode {
				callCtx.Errf("%s", noWritableRootHint)
			} else {
				callCtx.Errf("%s", readOnlyMessage)
			}
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: logrotate (-s SIZE|-f) [OPTION]... FILE...\n")
			callCtx.Out("Truncate each FILE to zero bytes through the AllowedPaths sandbox.\n")
			callCtx.Out("Symlinked write targets are rejected; pass the real log path instead.\n")
			callCtx.Out("This rshell subset does not parse config files, retain rotated copies,\n")
			callCtx.Out("compress logs, write state files, or run rotate scripts.\n\n")

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

		hasSize := fs.Changed("size")
		if !hasSize && !*force {
			callCtx.Errf("logrotate: you must specify --size or --force\n")
			return builtins.Result{Code: 1}
		}
		if hasSize && *force {
			callCtx.Errf("logrotate: --size and --force cannot be combined\n")
			return builtins.Result{Code: 1}
		}

		var threshold int64
		if hasSize {
			n, err := sizeparse.Parse(*sizeStr)
			if err != nil {
				if errors.Is(err, sizeparse.ErrRelative) {
					callCtx.Errf("logrotate: invalid size %q: %s\n", *sizeStr, err)
				} else {
					callCtx.Errf("logrotate: invalid size %q\n", *sizeStr)
				}
				return builtins.Result{Code: 1}
			}
			threshold = n
		}

		if len(files) == 0 {
			callCtx.Errf("logrotate: missing file operand\n")
			return builtins.Result{Code: 1}
		}

		report := *verbose || *dryRun
		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				return builtins.Result{Code: 1}
			}

			sizeBefore, truncated, err := callCtx.TruncateToZeroIfAtLeast(ctx, file, threshold, *dryRun)
			if err != nil {
				callCtx.Errf("logrotate: %q: %s\n", file, callCtx.PortableErr(err))
				failed = true
				continue
			}
			if !truncated {
				if report {
					if sizeBefore == 0 && threshold == 0 {
						callCtx.Outf("logrotate: %s: already empty, skipping\n", file)
					} else if *dryRun {
						callCtx.Outf("logrotate: %s: would skip, %d bytes below threshold %d\n", file, sizeBefore, threshold)
					} else {
						callCtx.Outf("logrotate: %s: %d bytes below threshold %d, skipping\n", file, sizeBefore, threshold)
					}
				}
				continue
			}
			if report {
				if *dryRun {
					callCtx.Outf("logrotate: %s: would truncate %d bytes\n", file, sizeBefore)
				} else {
					callCtx.Outf("logrotate: %s: truncated %d bytes\n", file, sizeBefore)
				}
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}
