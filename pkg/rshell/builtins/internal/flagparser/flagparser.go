// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package flagparser bridges between pflag and the GNU-getopt wording
// that rshell builtins are expected to match.
//
// pflag is a fine option-parsing library but its error wording diverges
// from GNU coreutils on every common failure mode (unknown flag,
// missing argument, no-arg flag given a value, etc.), and it has no
// notion of GNU's left-to-right `--help` short-circuit. Two helpers
// fix that without touching pflag itself:
//
//   - [RewriteError] translates pflag's error messages to GNU's
//     getopt-style wording, optionally consulting the original argv to
//     recover information pflag strips (e.g. the `=value` suffix on an
//     unknown long flag).
//
//   - [TrialHelpTrimIndex] decides whether the suffix after a `--help`
//     token can be safely discarded so the builtin's handler can
//     short-circuit. It trial-parses the prefix on a throw-away
//     FlagSet; if every preceding option parses cleanly the suffix is
//     dropped, otherwise the caller leaves the args alone and the real
//     parse surfaces the earlier error — matching GNU's leftmost-bad-
//     option rule.
//
// Keeping this code in a dedicated package isolates the parser-shim
// concern from builtin registration. If we ever replace pflag with a
// hand-written GNU-getopt parser, the swap is local to this package.
package flagparser

import (
	"io"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// RewriteError translates pflag's default error messages to the
// GNU-getopt-style wording that matches GNU coreutils byte-for-byte.
// It returns the rewritten message without a trailing newline or any
// "cmd:" prefix; callers prepend the builtin name themselves.
//
// args is the argv that was handed to fs.Parse. It is consulted only
// for the unknown-long-flag case, where pflag strips the `=value`
// suffix from its error string but GNU getopt reports the full token
// (`--no-such=value`, not just `--no-such`).
//
// Patterns covered:
//
//	pflag                                       → GNU
//	"unknown flag: --foo"                       → "unrecognized option '--foo'"
//	                                              ("--foo=value" if the original argv used =value)
//	"unknown shorthand flag: 'X' in -..."       → "invalid option -- 'X'"
//	"flag needs an argument: --foo"             → "option '--foo' requires an argument"
//	"flag needs an argument: 'X' in -Y"         → "option requires an argument -- 'X'"
//	`invalid argument "..." for "DESC" flag:    → "option '--LONG' doesn't allow
//	   flag does not allow an argument`              an argument"
//
// Unknown messages are returned unchanged.
func RewriteError(err error, args []string) string {
	msg := err.Error()

	// pflag wraps errors returned by a Var's Set(value) as
	//   `invalid argument "VALUE" for "DESC" flag: INNER`
	// where DESC is e.g. `-h, --human-readable` or `--total`. df's
	// noArgBool / unitFlag.Set returns the literal "flag does not
	// allow an argument" so users (and tests) see GNU's "doesn't"
	// instead of the wrapped pflag verbosity.
	if strings.HasPrefix(msg, "invalid argument ") &&
		strings.HasSuffix(msg, "flag does not allow an argument") {
		if d, ok := extractFlagDescriptor(msg); ok {
			// pflag's error doesn't tell us which form (long --foo=val
			// or short -X=val) the user actually typed; both route
			// through the same Set call. We need to know because GNU
			// emits different wording: `option '--foo' doesn't allow
			// an argument` for the long form vs `invalid option --
			// '='` for the short form (where GNU getopt sees `=` as
			// an invalid shorthand char following the registered one).
			//
			// pflag stops at the FIRST bad option in argv, so we scan
			// left-to-right and use whichever form appears first. Only
			// scan up to `--` (end-of-flags).
			long := longFlagName(d)
			short, hasShort := shortFlagFromDescriptor(d)
			hasLong := strings.HasPrefix(long, "--")
			longPrefix := long + "="
			for _, a := range args {
				if a == "--" {
					break
				}
				if hasLong && strings.HasPrefix(a, longPrefix) {
					return "option '" + long + "' doesn't allow an argument"
				}
				if hasShort && len(a) >= 3 && a[0] == '-' && a[1] != '-' && a[1] == short && a[2] == '=' {
					return "invalid option -- '='"
				}
			}
			// Neither form found in argv (shouldn't happen for a
			// successful pflag failure but stay safe). Default to the
			// long-form message.
			return "option '" + long + "' doesn't allow an argument"
		}
	}

	if rest, ok := strings.CutPrefix(msg, "unknown flag: "); ok {
		// pflag's error carries only the flag name; GNU's reports the
		// full token, so we recover `--foo=value` from argv when the
		// user wrote it that way.
		return "unrecognized option '" + safeDiagnosticText(recoverLongFlagToken(rest, args)) + "'"
	}

	const shortPrefix = "unknown shorthand flag: '"
	if rest, ok := strings.CutPrefix(msg, shortPrefix); ok {
		if char, _, ok := strings.Cut(rest, "'"); ok {
			return "invalid option -- '" + safeDiagnosticText(char) + "'"
		}
	}

	if rest, ok := strings.CutPrefix(msg, "flag needs an argument: "); ok {
		// pflag emits two distinct payloads depending on the form:
		//   long form:  `--foo`
		//   short form: `'X' in -Y` (X is the char, Y is the run
		//                            of shorthand letters)
		// GNU getopt uses different wording for each: long flags get
		// `option '--foo' requires an argument`, short flags get
		// `option requires an argument -- 'X'`.
		if char, found := shortMissingArg(rest); found {
			return "option requires an argument -- '" + safeDiagnosticText(char) + "'"
		}
		return "option '" + safeDiagnosticText(rest) + "' requires an argument"
	}

	if rest, ok := strings.CutPrefix(msg, "bad flag syntax: "); ok {
		return "bad flag syntax: " + safeDiagnosticText(rest)
	}

	return msg
}

// safeDiagnosticText escapes unsafe bytes before interpolation into an error.
func safeDiagnosticText(s string) string {
	quoted := strconv.QuoteToGraphic(s)
	return quoted[1 : len(quoted)-1]
}

// TrialHelpTrimIndex returns the index of the first `--help` in args
// if trial-parsing the prefix [0..helpIdx] with a fresh FlagSet
// (1) succeeds and (2) actually sets the `help` flag. The second
// check matters because pflag can consume the `--help` token in ways
// that don't set the flag:
//
//   - As the value of a preceding value-taker. `grep -e --help` makes
//     `--help` the pattern for -e, not a help request; the trial parse
//     accepts it but `Lookup("help").Changed` stays false.
//   - As a positional, for FlagSets that called `SetInterspersed(false)`
//     (xargs, tr, read). Anything after the first positional is left
//     for the builtin's handler — `--help` included.
//
// If the trial both parses and sets `help`, the suffix after `--help`
// is safely discardable and the builtin's handler can short-circuit —
// matching GNU coreutils' left-to-right semantics. Otherwise we
// return (0, false) so the real fs.Parse reports any later error and
// the handler sees the full argv.
//
// registerFlags is invoked on a throw-away FlagSet to set up the
// trial; it must be safe to call multiple times. Every builtin's
// MakeFlags is pure flag-registration today, so this is fine.
func TrialHelpTrimIndex(name string, registerFlags func(*pflag.FlagSet), args []string) (int, bool) {
	helpIdx := -1
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "--help" {
			helpIdx = i
			break
		}
	}
	if helpIdx < 0 {
		return 0, false
	}
	trial := pflag.NewFlagSet(name, pflag.ContinueOnError)
	trial.SetOutput(io.Discard)
	registerFlags(trial)
	if trial.Parse(args[:helpIdx+1]) != nil {
		return 0, false
	}
	helpFlag := trial.Lookup("help")
	if helpFlag == nil || !helpFlag.Changed {
		return 0, false
	}
	return helpIdx, true
}

// recoverLongFlagToken returns the original argv token for an unknown
// long flag whose stripped name (e.g. `--no-such`) appears in pflag's
// error. If the user wrote `--no-such=value`, the full token is
// returned; otherwise the bare flag is returned. Stops at `--` so a
// later positional like `-- --no-such=foo` is never misclassified.
func recoverLongFlagToken(flag string, args []string) string {
	prefix := flag + "="
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == flag || strings.HasPrefix(a, prefix) {
			return a
		}
	}
	return flag
}

// shortFlagFromDescriptor extracts the single shorthand char from a
// pflag descriptor like `-h, --human-readable`. Returns false for
// long-only descriptors (`--total`).
func shortFlagFromDescriptor(d string) (byte, bool) {
	if len(d) >= 2 && d[0] == '-' && d[1] != '-' {
		return d[1], true
	}
	return 0, false
}

// shortMissingArg parses pflag's short-form payload for
// `flag needs an argument: 'X' in -Y` and returns the bare char
// (`X`). The second return is false when payload is in the long-form
// (`--foo`), letting the caller fall through.
func shortMissingArg(payload string) (string, bool) {
	if !strings.HasPrefix(payload, "'") {
		return "", false
	}
	char, _, found := strings.Cut(payload[1:], "'")
	if !found {
		return "", false
	}
	return char, true
}

// extractFlagDescriptor parses pflag's `invalid argument "..." for
// "DESC" flag: ...` wrapper and returns the DESC segment.
func extractFlagDescriptor(msg string) (string, bool) {
	_, after, found := strings.Cut(msg, ` for "`)
	if !found {
		return "", false
	}
	desc, _, found := strings.Cut(after, `" flag:`)
	if !found {
		return "", false
	}
	return desc, true
}

// longFlagName returns the long-form (--name) flag name from a pflag
// descriptor like `-X, --LONG` or `--LONG`. For shorthand-only flags
// (rare; e.g. df's hidden `-k`) the descriptor is just `-X` and we
// return it unchanged.
func longFlagName(descriptor string) string {
	if i := strings.LastIndex(descriptor, ", "); i >= 0 {
		return descriptor[i+2:]
	}
	return descriptor
}
