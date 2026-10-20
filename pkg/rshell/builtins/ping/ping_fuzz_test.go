// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ping_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// cmdRunCtxFuzz runs a ping command with a caller-supplied context.
// Separate from runScriptCtx to avoid any accidental redeclarations.
func cmdRunCtxFuzz(ctx context.Context, t *testing.T, script string) (string, string, int) {
	t.Helper()
	return runScriptCtx(ctx, t, script)
}

// FuzzPingFlags fuzzes flag argument parsing. The command is always directed
// at an unresolvable host so DNS failure terminates the run quickly. The only
// acceptable exit codes are 0 and 1; any other code or panic is a failure.
func FuzzPingFlags(f *testing.F) {
	// Source A: implementation boundary values.
	f.Add("-c", "1")
	f.Add("-c", "4")
	f.Add("-c", "20")
	f.Add("-c", "0")
	f.Add("-c", "-1")
	f.Add("-c", "21")
	f.Add("-c", "2147483647")
	f.Add("-c", "9999999999999999999")
	f.Add("-W", "100ms")
	f.Add("-W", "1s")
	f.Add("-W", "30s")
	f.Add("-W", "0s")
	f.Add("-W", "31s")
	f.Add("-i", "200ms")
	f.Add("-i", "1s")
	f.Add("-i", "10ms")
	f.Add("-i", "0s")
	f.Add("-q", "")
	f.Add("-4", "")
	f.Add("-6", "")

	// Source B: CVE-class and historical boundary inputs.
	f.Add("-c", "4294967295")  // UINT32_MAX
	f.Add("-c", "-2147483648") // INT32_MIN
	f.Add("-W", "1000000000s") // absurdly large duration
	f.Add("-i", "1ns")         // below minimum floor

	// Source C: flag strings derived from test coverage.
	f.Add("--count", "5")
	f.Add("--wait", "2s")
	f.Add("--interval", "500ms")
	f.Add("--quiet", "")
	f.Add("--help", "")
	f.Add("-h", "")

	f.Fuzz(func(t *testing.T, flag, value string) {
		if t.Context().Err() != nil {
			return
		}
		// Only allow characters that are safe to pass unquoted in a shell script.
		// Using an allowlist is more robust than a denylist: any character not
		// explicitly permitted here could cause shell parse errors or command
		// injection, so we skip instead of risk a spurious test failure.
		for _, r := range flag + value {
			safe := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '.'
			if !safe {
				return
			}
		}
		if len(flag)+len(value) > 256 {
			return
		}

		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()

		var script string
		if value == "" {
			script = fmt.Sprintf("ping %s no-such-host-xyzzy.invalid", flag)
		} else {
			script = fmt.Sprintf("ping %s %s no-such-host-xyzzy.invalid", flag, value)
		}

		_, _, code := cmdRunCtxFuzz(ctx, t, script)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for script: %s", code, script)
		}
	})
}

// FuzzPingHostname fuzzes the hostname argument. The command runs with minimal
// packet count (-c 1 -W 500ms) to fail fast on any input.
func FuzzPingHostname(f *testing.F) {
	// Source A: boundary and edge-case hostnames.
	f.Add("localhost")
	f.Add("127.0.0.1")
	f.Add("::1")
	f.Add("")
	f.Add("no-such-host.invalid")
	f.Add("a")
	f.Add("0.0.0.0")
	f.Add("255.255.255.255")

	// Source B: historically problematic inputs (null bytes, long strings, unicode).
	f.Add(strings.Repeat("a", 253))   // max FQDN length
	f.Add(strings.Repeat("a", 254))   // over max FQDN length
	f.Add(strings.Repeat("a", 10000)) // very long
	f.Add("192.0.2.1")                // TEST-NET (RFC 5737), unroutable
	f.Add("198.51.100.1")             // TEST-NET-2, unroutable
	f.Add("203.0.113.1")              // TEST-NET-3, unroutable
	f.Add("xn--nxasmq6b.com")         // IDN / punycode
	f.Add("\x00\x00\x00\x00")         // null bytes
	f.Add("a..b")                     // double dot
	f.Add("-hostname")                // leading dash

	// Source C: from test coverage.
	f.Add("no-such-host-xyzzy.invalid")

	f.Fuzz(func(t *testing.T, hostname string) {
		if t.Context().Err() != nil {
			return
		}
		// Only allow characters that are safe to pass unquoted as a shell
		// argument. An allowlist is more robust than a denylist because the
		// shell parser has many special characters and we cannot enumerate
		// them all in advance. Valid hostname characters are: letters, digits,
		// hyphens, dots (domain labels), and colons (IPv6 addresses).
		for _, r := range hostname {
			safe := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '.' || r == ':'
			if !safe {
				return
			}
		}
		if len(hostname) > 10000 {
			return
		}
		if hostname == "" {
			return // empty hostname is handled by TestPingPentestEmptyHostname
		}

		// 1s is enough for fast DNS + socket attempt; shorter than the
		// 3s default to keep CI fuzz runs from stalling on unresolvable
		// hostnames when the corpus grows to include slow-DNS entries.
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		script := fmt.Sprintf("ping -c 1 -W 500ms -- %s", hostname)
		_, _, code := cmdRunCtxFuzz(ctx, t, script)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for hostname: %q", code, hostname)
		}
	})
}
