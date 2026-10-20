// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procfd

import "testing"

// FuzzParseComm feeds arbitrary /proc/<pid>/stat content to parseComm.
// MUST NOT panic or loop indefinitely regardless of parenthesis placement,
// nesting, or malformed field counts — the comm field itself is fully
// attacker-influenced (a process can name itself almost anything via
// PR_SET_NAME/argv[0], including further parentheses).
func FuzzParseComm(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("123 (bash) S 1"))
	f.Add([]byte("123 (bash) S 1\n"))
	f.Add([]byte("123 bash S 1"))
	f.Add([]byte("123 (bash S 1"))
	f.Add([]byte("123 bash) S 1"))
	f.Add([]byte("123 () S 1"))
	f.Add([]byte("123 (()) S 1"))
	f.Add([]byte("123 (a(b)c) S 1"))
	f.Add([]byte("123 (a(b(c)d)e) S 1"))
	f.Add([]byte("(((((((((("))
	f.Add([]byte(")))))))))))"))
	f.Add([]byte("123 (name with spaces and ) parens ( inside) S 1"))
	f.Add([]byte("123 (\x00embedded nul) S 1"))
	f.Add([]byte("\xff\xfe(bad utf8)\n"))
	f.Add([]byte(")("))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		comm, err := parseComm(data)
		if err == nil && comm == "" {
			// A successful parse with balanced, non-empty parens can still
			// legitimately yield "" (e.g. "123 () S 1"); only guard against
			// a panic, already covered by f.Fuzz's recover.
			return
		}
	})
}

// FuzzReadUIDFromStatus feeds arbitrary /proc/<pid>/status content to
// parseUIDFromStatus. MUST NOT panic and MUST always return a non-empty
// string ("?" when no Uid line is found or parseable).
func FuzzReadUIDFromStatus(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("Name:\tbash\n"))
	f.Add([]byte("Name:\tbash\nUid:\t1000\t1000\t1000\t1000\n"))
	f.Add([]byte("Uid:\n"))
	f.Add([]byte("Uid:\t\n"))
	f.Add([]byte("Uid:\tnotanumber\n"))
	f.Add([]byte("Uid:\t0\n"))
	f.Add([]byte("Uid:\t-1\n"))
	f.Add([]byte("\x00\x00\x00Uid:\t1000\n"))
	f.Add([]byte("Uid:\t1000\r\nUid:\t2000\n"))
	// Exceeds the 1<<20 scanner.Buffer cap with no newline: must surface as
	// a scan failure (returning "?"), not a panic or unbounded allocation.
	f.Add(longLine(2 << 20))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4<<20 {
			return
		}
		got := parseUIDFromStatus(data)
		if got == "" {
			t.Fatalf("parseUIDFromStatus returned empty string for input %q, want at least \"?\"", data)
		}
	})
}

// longLine builds a single long line with no Uid: prefix, exercising the
// bufio.Scanner buffer-growth path (capped at 1<<20 by readUID's
// scanner.Buffer call) without a real Uid match.
func longLine(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return b
}

// FuzzIsRealPath guards the memfd-prefix bypass fix: no input, however
// crafted, should be misclassified in a way that lets a non-path target
// masquerade as IsPath=false while starting with "/", or vice versa. The
// property under test is simply "IsPath tracks the leading slash exactly",
// which is what closes the previously-removed "/memfd:" special case.
func FuzzIsRealPath(f *testing.F) {
	f.Add("/tmp/foo")
	f.Add("socket:[12345]")
	f.Add("pipe:[12345]")
	f.Add("anon_inode:[eventfd]")
	f.Add("memfd:secret")
	f.Add("/memfd:secret")
	f.Add("")
	f.Add("/")
	f.Add("//")
	f.Add(" /leading-space-not-slash")
	f.Add("\x00/nul-then-slash")

	f.Fuzz(func(t *testing.T, target string) {
		if len(target) > 1<<16 {
			return
		}
		got := isRealPath(target)
		want := len(target) > 0 && target[0] == '/'
		if got != want {
			t.Fatalf("isRealPath(%q) = %v, want %v (leading-slash check diverged)", target, got, want)
		}
	})
}
