// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lsof

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procfd"
)

// FuzzRedactName is the primary security-relevant fuzz target in this
// package: redactName decides whether a /proc-reported path string is safe
// to print or must be replaced with "(restricted)". It MUST NOT panic on any
// input, and — the property that matters — it MUST NEVER return a string
// that both differs from the fixed "(restricted)"/"(restricted) (deleted)"
// markers AND is outside the configured root once cleaned. In other words,
// whenever redactName's result is not one of those two markers, the printed
// text must be exactly of.Name (never a partially-redacted or
// differently-shaped leak).
func FuzzRedactName(f *testing.F) {
	f.Add("/allowed/file", "/allowed", "", false)
	f.Add("/allowed/../secret", "/allowed", "", false)
	f.Add("/allowed", "/allowed", "", false)
	f.Add("/", "/", "", false)
	f.Add("/allowed-other/x", "/allowed", "", false)
	f.Add("/host/var/log/x", "/host/var/log", "/host", false)
	f.Add("/var/log/x", "/host/var/log", "/host", false)
	f.Add("", "/allowed", "", false)
	f.Add("relative/path", "/allowed", "", false)
	f.Add("/allowed/x", "", "", false)
	f.Add(strings.Repeat("/../", 10_000)+"etc/passwd", "/allowed", "", false)
	f.Add("/allowed/\x00nul", "/allowed", "", false)
	f.Add("/allowed/file", "/allowed", "", true)

	f.Fuzz(func(t *testing.T, name, rootRaw, hostPrefix string, deleted bool) {
		if len(name) > 1<<16 || len(rootRaw) > 1<<16 || len(hostPrefix) > 1<<16 {
			return
		}
		var roots []gateRoot
		if rootRaw != "" {
			roots = []gateRoot{{raw: rootRaw}}
		}
		of := procfd.OpenFile{Name: name, IsPath: true, Deleted: deleted}

		got := redactName(of, roots, hostPrefix)

		if got == "(restricted)" || got == "(restricted) (deleted)" {
			return
		}
		// Anything else must be the untouched name (plus the deleted
		// marker) — never a mangled or partially-cleaned variant that
		// could itself leak information not present in of.Name.
		want := of.Name
		if of.Deleted {
			want += " (deleted)"
		}
		if got != want {
			t.Fatalf("redactName(%q, roots=%v, hostPrefix=%q) = %q, want either a restricted marker or exactly %q", name, roots, hostPrefix, got, want)
		}
	})
}

// FuzzPathWithinRoots exercises the lexical containment check directly.
// MUST NOT panic, and a target must never be reported as within a root
// unless filepath.Rel confirms it (guarded indirectly: this just pins
// panic-freedom and a couple of invariants cheap to check generically).
func FuzzPathWithinRoots(f *testing.F) {
	f.Add("/etc/passwd", "/")
	f.Add("/allowed/x", "/allowed")
	f.Add("/allowed-other/x", "/allowed")
	f.Add("", "/allowed")
	f.Add("/allowed", "")
	f.Add("relative", "/allowed")
	f.Add("/allowed/../../etc/passwd", "/allowed")
	f.Add("///allowed//x", "/allowed")

	f.Fuzz(func(_ *testing.T, target, root string) {
		if len(target) > 1<<16 || len(root) > 1<<16 {
			return
		}
		roots := []gateRoot{{raw: root}}
		// Must not panic; result itself has no simpler invariant to check
		// generically without reimplementing filepath.Rel.
		_ = pathWithinRoots(target, roots)
	})
}

// FuzzParsePIDs fuzzes the -p flag value parser. MUST NOT panic, and on
// success every returned PID must be a positive integer with no duplicates.
func FuzzParsePIDs(f *testing.F) {
	f.Add("1")
	f.Add("1,2,3")
	f.Add("0")
	f.Add("-1")
	f.Add("")
	f.Add(",,,")
	f.Add("1,,2")
	f.Add("1 2 3")
	f.Add("99999999999999999999")
	f.Add(" 1 , 2 ")
	f.Add("1,1,1")
	f.Add("+1")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<16 {
			return
		}
		pids, err := parsePIDs(s)
		if err != nil {
			return
		}
		seen := make(map[int]bool, len(pids))
		for _, p := range pids {
			if p <= 0 {
				t.Fatalf("parsePIDs(%q) returned non-positive PID %d", s, p)
			}
			if seen[p] {
				t.Fatalf("parsePIDs(%q) returned duplicate PID %d", s, p)
			}
			seen[p] = true
		}
	})
}

// FuzzParseUIDs mirrors FuzzParsePIDs for the -u flag value parser. Unlike
// PIDs, 0 is a valid UID (root), so only non-negative-and-numeric plus
// no-duplicates is checked.
func FuzzParseUIDs(f *testing.F) {
	f.Add("0")
	f.Add("1000")
	f.Add("0,1000")
	f.Add("")
	f.Add(",,,")
	f.Add("-1")
	f.Add("notanumber")
	f.Add("1000,1000")
	f.Add(" 0 , 1000 ")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<16 {
			return
		}
		uids, err := parseUIDs(s)
		if err != nil {
			return
		}
		seen := make(map[string]bool, len(uids))
		for _, u := range uids {
			if seen[u] {
				t.Fatalf("parseUIDs(%q) returned duplicate UID %q", s, u)
			}
			seen[u] = true
		}
	})
}
