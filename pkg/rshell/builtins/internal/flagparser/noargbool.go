// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package flagparser

import (
	"errors"

	"github.com/spf13/pflag"
)

// NoArgSentinel is the NoOptDefVal pflag passes to a no-argument flag's
// Set method when the user writes the bare flag (`-h`, `--all`); for the
// explicit-value form (`--all=true`) pflag passes the user's literal
// value instead.
//
// We use a single NUL byte so the two cases are distinguishable: NUL is
// the C-string terminator and POSIX execve(2) refuses to pass argv
// elements containing it, so the user cannot forge this string from a
// shell. That lets Set reject every `=value` form — including `=true` —
// to match GNU getopt's "doesn't allow an argument" exit-1 error.
const NoArgSentinel = "\x00"

// noArgBool is a pflag.Value that mirrors GNU getopt's no_argument
// behaviour: bare `--flag` and `-f` work, but `--flag=value` and
// `-f=value` are rejected with "flag does not allow an argument" for
// every value (including `=true`). pflag.BoolP treats the explicit-value
// form as a successful parse, which silently diverges from GNU.
type noArgBool struct {
	target *bool
}

func (b *noArgBool) String() string {
	if b.target != nil && *b.target {
		return "true"
	}
	return "false"
}
func (b *noArgBool) Type() string { return "bool" }
func (b *noArgBool) Set(s string) error {
	if s != NoArgSentinel {
		return errors.New("flag does not allow an argument")
	}
	*b.target = true
	return nil
}

// RegisterNoArgBool installs a no-argument boolean flag on fs and returns
// the *bool target so the caller can read it like an ordinary fs.Bool
// result. Pass an empty shorthand for long-only flags (e.g. --total).
//
// Unlike fs.BoolP, an explicit value (`--flag=true`, `--flag=false`,
// `-f=x`) is rejected with GNU's "doesn't allow an argument" wording
// (via [RewriteError]) instead of silently parsing — matching GNU
// getopt's no_argument semantics for flags where accepting a stray value
// would be surprising or unsafe (e.g. rm's --help/--verbose).
func RegisterNoArgBool(fs *pflag.FlagSet, name, shorthand, usage string) *bool {
	target := new(bool)
	flag := fs.VarPF(&noArgBool{target: target}, name, shorthand, usage)
	flag.NoOptDefVal = NoArgSentinel
	return target
}
