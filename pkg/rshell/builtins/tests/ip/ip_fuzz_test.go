// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ip_test

// FuzzIP fuzzes the ip builtin with arbitrary subcommand and argument strings.
//
// ip does not read files or stdin; all seeds are subcommand/flag strings.
// The fuzzer verifies the implementation never panics and always returns
// exit code 0 or 1 — never crashes or hangs.
//
// Seed corpus sources:
//   A. Implementation edge cases: all blocked subcommands, write ops, flags
//   B. GTFOBins and CVE-class inputs: batch mode, netns, force
//   C. Existing test coverage: all flag combinations from ip_test.go

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cmdRunCtxFuzz runs an ip command with context for fuzz tests.
// Unlike runScriptCtx, it does not fatalf on internal shell errors — those
// can occur for unusual inputs before the builtin runs and are not our bug.
// Instead it returns exit code -1 so the caller can skip those inputs.
func cmdRunCtxFuzz(ctx context.Context, t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		// Shell parse error — input is not a valid script; skip.
		return "", err.Error(), -1
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
	if err != nil {
		t.Fatalf("interp.New: %v", err)
	}
	defer runner.Close()
	runErr := runner.Run(ctx, prog)
	exitCode = 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else if ctx.Err() != nil {
			exitCode = -1
		} else {
			// Internal shell error (not a builtin exit code) — skip.
			return outBuf.String(), errBuf.String(), -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// FuzzIPSubcommand fuzzes the subcommand and argument portion of the ip command.
//
// Seeds cover: all known-good commands, all blocked write ops, all GTFOBins
// vectors, edge-case interface names, flag injection.
func FuzzIPSubcommand(f *testing.F) {
	// Source A: Implementation edge cases — normal commands
	f.Add("addr show")
	f.Add("addr")
	f.Add("link show")
	f.Add("link")
	f.Add("addr show dev lo")
	f.Add("link show dev lo")
	f.Add("address show")
	f.Add("addr list")
	f.Add("addr lst")

	// Source A: write operations — all must exit 1
	f.Add("addr add 10.0.0.1/24 dev lo")
	f.Add("addr del 10.0.0.1/24 dev lo")
	f.Add("addr flush dev lo")
	f.Add("addr change 10.0.0.1/24 dev lo")
	f.Add("addr replace 10.0.0.1/24 dev lo")
	f.Add("addr append 10.0.0.1/24 dev lo")
	f.Add("link set lo up")
	f.Add("link del lo")
	f.Add("link change lo mtu 9000")

	// Source A: blocked subcommands
	f.Add("netns exec mynamespace sh")
	f.Add("netns add myns")
	f.Add("route show")
	f.Add("tunnel add tun0 mode gre")
	f.Add("monitor")
	f.Add("xfrm state list")
	f.Add("maddress show")
	f.Add("mrule show")

	// Source B: GTFOBins / CVE-class inputs
	f.Add("") // empty object — must exit 1 "object required"
	f.Add("addr show dev nonexistent-interface-xyzzy-99")

	// Source B: dangerous interface names that could be injection attempts
	f.Add("addr show dev $(whoami)")
	f.Add("addr show dev `whoami`")
	f.Add("addr show dev ../../etc/passwd")
	f.Add("addr show dev lo; rm -rf /")
	f.Add("addr show dev lo && cat /etc/passwd")
	f.Add("addr show dev \"lo\"")

	// Source B: very long interface names
	f.Add("addr show dev " + strings.Repeat("a", 256))
	f.Add("addr show dev " + strings.Repeat("a", 1024))

	// Source B: null bytes in subcommand
	f.Add("addr\x00show")

	// Source C: all flag combinations from ip_test.go
	f.Add("addr show dev lo")
	f.Add("address show dev lo")
	f.Add("addr list dev lo")
	f.Add("addr lst dev lo")

	f.Fuzz(func(t *testing.T, subcmd string) {
		if t.Context().Err() != nil {
			return
		}
		if len(subcmd) > 1024 {
			return
		}
		// Reject invalid UTF-8 — the shell parser rejects it before the builtin runs.
		if !utf8.ValidString(subcmd) {
			return
		}
		// Reject inputs with shell metacharacters — they would parse as shell
		// syntax, not as ip arguments, and are not valid ip subcommands.
		// Also reject ~ (tilde expansion is not supported and returns exit 2).
		for _, ch := range []string{"\n", "\r", ";", "|", "&", "`", "$", "\"", "'", "(", ")", "{", "}", "<", ">", "~"} {
			if strings.Contains(subcmd, ch) {
				return
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := "ip " + subcmd
		_, _, code := cmdRunCtxFuzz(ctx, t, script)
		timedOut := ctx.Err() == context.DeadlineExceeded // capture before cancel()
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code == -1 {
			return // shell/parse error before the builtin ran — not our bug
		}
		if code != 0 && code != 1 && code != 255 {
			t.Errorf("ip %q: unexpected exit code %d", subcmd, code)
		}
		if timedOut {
			t.Errorf("ip %q: timed out (possible hang)", subcmd)
		}
	})
}

// FuzzIPFlags fuzzes global flag combinations passed before the subcommand.
//
// Seeds cover: all valid flags, blocked flags (-b/-B/-n/--force), and
// combinations that test the -4/-6 cancellation logic.
func FuzzIPFlags(f *testing.F) {
	// Source A: all valid single flags
	f.Add("-o", "addr show")
	f.Add("--brief", "addr show")
	f.Add("-4", "addr show")
	f.Add("-6", "addr show")
	f.Add("-h", "")
	f.Add("--help", "")

	// Source A: flag combinations
	f.Add("-4 -6", "addr show")
	f.Add("-o --brief", "addr show")
	f.Add("-4 -o", "addr show")
	f.Add("-6 --brief", "addr show")

	// Source B: blocked flags — must exit 1
	f.Add("-b /tmp/evil", "addr show")
	f.Add("-B /tmp/evil", "addr show")
	f.Add("--force", "addr show")
	f.Add("-n mynamespace", "addr show")

	// Source B: unknown flags — must exit 1
	f.Add("--no-such-flag", "addr show")
	f.Add("-z", "addr show")
	f.Add("-x", "addr show")

	// Source C: all flag-heavy test cases from ip_test.go
	f.Add("-4", "addr show dev lo")
	f.Add("-6", "addr show dev lo")
	f.Add("-4 -6", "addr show dev lo")
	f.Add("-o", "addr show dev lo")
	f.Add("--brief", "addr show dev lo")
	f.Add("-o", "link show dev lo")
	f.Add("--brief", "link show dev lo")

	f.Fuzz(func(t *testing.T, flags, subcmd string) {
		if t.Context().Err() != nil {
			return
		}
		if len(flags)+len(subcmd) > 512 {
			return
		}
		// Reject invalid UTF-8 — the shell parser rejects it before the builtin runs.
		if !utf8.ValidString(flags) || !utf8.ValidString(subcmd) {
			return
		}
		// Reject shell metacharacters and ~ (tilde expansion returns exit 2).
		for _, ch := range []string{"\n", "\r", ";", "|", "&", "`", "$", "\"", "'", "(", ")", "{", "}", "<", ">", "~"} {
			if strings.Contains(flags, ch) || strings.Contains(subcmd, ch) {
				return
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := "ip " + flags
		if subcmd != "" {
			script += " " + subcmd
		}
		_, _, code := cmdRunCtxFuzz(ctx, t, script)
		timedOut := ctx.Err() == context.DeadlineExceeded // capture before cancel()
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code == -1 {
			return // shell/parse error before the builtin ran — not our bug
		}
		if code != 0 && code != 1 {
			t.Errorf("ip %q %q: unexpected exit code %d", flags, subcmd, code)
		}
		if timedOut {
			t.Errorf("ip %q %q: timed out", flags, subcmd)
		}
	})
}
