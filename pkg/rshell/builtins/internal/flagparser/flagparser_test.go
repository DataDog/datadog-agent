// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package flagparser

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"
)

func TestTrialHelpTrimIndex(t *testing.T) {
	// registerFlags installs a representative set of flag shapes on
	// each fresh FlagSet: bool no-arg long+short, single-char no-arg
	// short, string value-taker. This mirrors how real builtins
	// compose flags and exercises every classification path in trial
	// parsing.
	registerFlags := func(fs *pflag.FlagSet) {
		fs.BoolP("help", "", false, "")
		fs.BoolP("verbose", "v", false, "")
		fs.BoolP("quiet", "q", false, "")
		fs.StringP("name", "n", "", "")
		fs.StringP("pattern", "e", "", "")
	}
	// registerFlagsNoIntersperse mirrors xargs/tr/read: pflag stops
	// parsing flags as soon as it sees the first positional.
	registerFlagsNoIntersperse := func(fs *pflag.FlagSet) {
		fs.SetInterspersed(false)
		fs.BoolP("help", "", false, "")
		fs.BoolP("verbose", "v", false, "")
	}

	tests := []struct {
		name    string
		args    []string
		wantIdx int
		wantOK  bool
	}{
		{"--help first", []string{"--help"}, 0, true},
		{"--help after no-arg long", []string{"--verbose", "--help", "--bogus"}, 1, true},
		{"--help after no-arg short", []string{"-q", "--help", "--bogus"}, 1, true},
		{"--help after cluster of no-arg shorts", []string{"-qv", "--help", "--bogus"}, 1, true},
		// Value-takers are now safe: pflag accepts any string in Set,
		// and builtins are expected to validate before --help fires.
		{"--help after value-taker (long form)", []string{"-n", "5", "--help"}, 2, true},
		{"--help after value-taker (any string)", []string{"-n", "nope", "--help"}, 2, true},
		{"--help after =value (valid)", []string{"--name=foo", "--help"}, 1, true},
		// `-n --help` — pflag consumes `--help` as `-n`'s value, so
		// the trial parses but `help` is never set. We MUST NOT trim,
		// otherwise the suffix `--bogus` would be silently dropped
		// even though no help was actually requested.
		{"--help consumed as value-taker's value", []string{"-n", "--help", "--bogus"}, 0, false},
		// `grep -e --help --bogus`: `-e` makes `--help` its pattern
		// value. Same reasoning — trial accepts but help.Changed is
		// false, so we leave args alone and pflag fails on --bogus
		// (matches GNU's `unrecognized option '--bogus'`).
		{"-e value-taker swallows --help", []string{"-e", "--help", "--bogus"}, 0, false},
		// Errors at parse time keep the suffix unstripped so the real
		// parse surfaces the same error.
		{"--help after unknown flag is unsafe", []string{"--bogus", "--help"}, 0, false},
		{"--help after --bool=garbage is unsafe", []string{"--verbose=garbage", "--help"}, 0, false},
		// `--` ends option parsing, so any `--help` after it is a
		// positional. We must not trim.
		{"--help after -- is unsafe", []string{"--", "--help"}, 0, false},
		// Positionals before --help are fine for pflag (they're just
		// collected) so trim is allowed.
		{"--help after positional", []string{"foo", "--help"}, 1, true},
		{"no --help in args", []string{"--verbose"}, 0, false},
		{"empty args", nil, 0, false},
		// Multi-byte UTF-8 in a shorthand cluster MUST not crash: pflag
		// would panic on a multi-byte ShorthandLookup, but trial.Parse
		// errors cleanly with "unknown shorthand flag", which we return
		// as (0, false). Regression for FuzzPwdArgs/9386e59311458487.
		{"non-ASCII byte in cluster", []string{"-˞", "--help"}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, ok := TrialHelpTrimIndex("t", registerFlags, tt.args)
			if idx != tt.wantIdx || ok != tt.wantOK {
				t.Errorf("TrialHelpTrimIndex(%v) = (%d, %v), want (%d, %v)", tt.args, idx, ok, tt.wantIdx, tt.wantOK)
			}
		})
	}

	// SetInterspersed(false) — xargs/tr/read style. Anything after
	// the first positional must NOT be treated as a flag, so a
	// `--help` past a positional must not trigger the trim.
	t.Run("SetInterspersed(false): --help after positional is positional", func(t *testing.T) {
		idx, ok := TrialHelpTrimIndex("t", registerFlagsNoIntersperse, []string{"echo", "--help", "--bogus"})
		if idx != 0 || ok {
			t.Errorf("got (%d, %v), want (0, false)", idx, ok)
		}
	})
	t.Run("SetInterspersed(false): leading --help still trims", func(t *testing.T) {
		idx, ok := TrialHelpTrimIndex("t", registerFlagsNoIntersperse, []string{"--help", "echo", "--bogus"})
		if idx != 0 || !ok {
			t.Errorf("got (%d, %v), want (0, true)", idx, ok)
		}
	})
}

func TestRewriteError(t *testing.T) {
	tests := []struct {
		name string
		in   string
		args []string
		want string
	}{
		{
			name: "unknown long flag",
			in:   "unknown flag: --no-such-flag",
			args: []string{"--no-such-flag"},
			want: "unrecognized option '--no-such-flag'",
		},
		{
			name: "unknown long flag with =value preserved",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such=value"},
			want: "unrecognized option '--no-such=value'",
		},
		{
			name: "unknown long flag with empty =",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such="},
			want: "unrecognized option '--no-such='",
		},
		{
			name: "unknown long flag — args lookup stops at --",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such", "--", "--no-such=other"},
			want: "unrecognized option '--no-such'",
		},
		{
			name: "unknown long flag escapes newline",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such=bad\nforged"},
			want: `unrecognized option '--no-such=bad\nforged'`,
		},
		{
			name: "unknown long flag escapes terminal and bidi controls",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such=\x1b\u202e"},
			want: `unrecognized option '--no-such=\x1b\u202e'`,
		},
		{
			name: "unknown long flag escapes invalid UTF-8",
			in:   "unknown flag: --no-such",
			args: []string{"--no-such=" + string([]byte{0xff})},
			want: `unrecognized option '--no-such=\xff'`,
		},
		{
			name: "bad flag syntax escapes terminal controls and newlines",
			in:   "bad flag syntax: ---A\x1b[31mB\nforged: line",
			args: []string{"---A\x1b[31mB\nforged: line"},
			want: `bad flag syntax: ---A\x1b[31mB\nforged: line`,
		},
		{
			name: "unknown shorthand single",
			in:   "unknown shorthand flag: 'X' in -X",
			args: []string{"-X"},
			want: "invalid option -- 'X'",
		},
		{
			name: "unknown shorthand within group",
			in:   "unknown shorthand flag: 'X' in -aX",
			args: []string{"-aX"},
			want: "invalid option -- 'X'",
		},
		{
			name: "missing argument long",
			in:   "flag needs an argument: --type",
			args: []string{"--type"},
			want: "option '--type' requires an argument",
		},
		{
			name: "missing argument short single",
			in:   "flag needs an argument: 'n' in -n",
			args: []string{"-n"},
			want: "option requires an argument -- 'n'",
		},
		{
			name: "missing argument short combined",
			in:   "flag needs an argument: 'n' in -an",
			args: []string{"-an"},
			want: "option requires an argument -- 'n'",
		},
		{
			name: "no-arg with long+short descriptor",
			in:   `invalid argument "true" for "-a, --all" flag: flag does not allow an argument`,
			args: []string{"--all=true"},
			want: "option '--all' doesn't allow an argument",
		},
		{
			name: "shorthand =value form maps to GNU's invalid =",
			in:   `invalid argument "true" for "-h, --human-readable" flag: flag does not allow an argument`,
			args: []string{"-h=true"},
			want: "invalid option -- '='",
		},
		{
			name: "shorthand =value form requires shorthand match",
			in:   `invalid argument "true" for "-a, --all" flag: flag does not allow an argument`,
			args: []string{"--all=true"},
			want: "option '--all' doesn't allow an argument",
		},
		{
			// Long form appears FIRST — even though a matching short form
			// is also in argv, pflag failed on the long one. We must not
			// report the (later-appearing) shorthand=value error.
			name: "long --foo=val before -X=val: long-form error wins",
			in:   `invalid argument "true" for "-h, --human-readable" flag: flag does not allow an argument`,
			args: []string{"--human-readable=true", "-h=x"},
			want: "option '--human-readable' doesn't allow an argument",
		},
		{
			// Short form appears FIRST — even if a later --LONG=val
			// exists in argv. (Not realistic for real builtins, but tests
			// the left-to-right rule.)
			name: "short -X=val before --LONG=val: short-form error wins",
			in:   `invalid argument "true" for "-h, --human-readable" flag: flag does not allow an argument`,
			args: []string{"-h=x", "--human-readable=true"},
			want: "invalid option -- '='",
		},
		{
			name: "no-arg with long-only descriptor",
			in:   `invalid argument "true" for "--total" flag: flag does not allow an argument`,
			args: []string{"--total=true"},
			want: "option '--total' doesn't allow an argument",
		},
		{
			name: "no-arg with short-only descriptor — argv used -X=value",
			in:   `invalid argument "true" for "-k" flag: flag does not allow an argument`,
			args: []string{"-k=true"},
			want: "invalid option -- '='",
		},
		{
			name: "wrapped Var.Set error not the no-arg case is left alone",
			in:   `invalid argument "z" for "-t, --radix" flag: invalid radix`,
			args: []string{"-t", "z"},
			want: `invalid argument "z" for "-t, --radix" flag: invalid radix`,
		},
		{
			name: "unknown pattern passes through",
			in:   "some custom error",
			args: nil,
			want: "some custom error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteError(errors.New(tt.in), tt.args)
			if got != tt.want {
				t.Errorf("RewriteError(%q, %v) = %q, want %q", tt.in, tt.args, got, tt.want)
			}
		})
	}
}
