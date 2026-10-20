// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lsof

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procfd"
)

// TestPathWithinRootsRootSlash verifies that a root of "/" (granting full
// filesystem visibility) matches every absolute path. A naive
// rc+separator prefix check turns "/" into "//", which nothing but the
// literal path "/" would ever match — this was the root-"/" bug.
func TestPathWithinRootsRootSlash(t *testing.T) {
	roots := []gateRoot{{raw: "/"}}
	if !pathWithinRoots("/etc/passwd", roots) {
		t.Error("expected /etc/passwd to be within root \"/\"")
	}
	if !pathWithinRoots("/", roots) {
		t.Error("expected \"/\" itself to be within root \"/\"")
	}
}

// TestPathWithinRootsCanonicalAlias verifies that a target matching a
// root's canonical (symlink-resolved) form is accepted even though it does
// not match the root's raw configured spelling — the case where an
// AllowedPaths root is itself a symlink.
func TestPathWithinRootsCanonicalAlias(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed-symlink", canonical: "/real/target"}}
	if !pathWithinRoots("/real/target/file", roots) {
		t.Error("expected a path under the canonical alias to match")
	}
	if pathWithinRoots("/other/file", roots) {
		t.Error("expected an unrelated path to be rejected")
	}
}

// TestPathWithinRootsPrefixCollisionStillRejected guards against a
// regression of the OTHER prefix scenario (distinct from the root-"/"
// bug): a root of "/allowed" must not match "/allowed-other/foo".
func TestPathWithinRootsPrefixCollisionStillRejected(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed"}}
	if pathWithinRoots("/allowed-other/foo", roots) {
		t.Error("expected /allowed-other/foo to NOT match root /allowed")
	}
}

// TestRedactNameAppliesHostPrefix verifies that a HostPrefix-configured
// sandbox translates a /proc-reported path before the AllowedPaths check,
// matching pwd.go's/cd.go's resolveSymlinks handling of the same
// translation. Without it, every genuinely-allowed path under a
// HostPrefix-configured sandbox would be incorrectly redacted.
func TestRedactNameAppliesHostPrefix(t *testing.T) {
	roots := []gateRoot{{raw: "/host/var/log"}}
	of := procfd.OpenFile{Name: "/var/log/app.log", IsPath: true}
	if got := redactName(of, roots, "/host"); got != of.Name {
		t.Errorf("redactName = %q, want unredacted %q", got, of.Name)
	}
}

func TestRedactNameNoHostPrefixConfigured(t *testing.T) {
	roots := []gateRoot{{raw: "/var/log"}}
	of := procfd.OpenFile{Name: "/var/log/app.log", IsPath: true}
	if got := redactName(of, roots, ""); got != of.Name {
		t.Errorf("redactName = %q, want unredacted %q", got, of.Name)
	}
}

func TestRedactNameNonPathNeverGated(t *testing.T) {
	of := procfd.OpenFile{Name: "socket:[12345]", IsPath: false}
	if got := redactName(of, nil, ""); got != of.Name {
		t.Errorf("redactName = %q, want unredacted non-path %q", got, of.Name)
	}
}

func TestRedactNameOutsideRootsRestricted(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed"}}
	of := procfd.OpenFile{Name: "/other/file", IsPath: true}
	if got := redactName(of, roots, ""); got != "(restricted)" {
		t.Errorf("redactName = %q, want \"(restricted)\"", got)
	}
	ofDeleted := procfd.OpenFile{Name: "/other/file", IsPath: true, Deleted: true}
	if got := redactName(ofDeleted, roots, ""); got != "(restricted) (deleted)" {
		t.Errorf("redactName = %q, want \"(restricted) (deleted)\"", got)
	}
}

// TestRedactNameDotDotTraversalNeutralized guards the same property as
// TestLsofPentestDotDotTraversalCannotEscapeGating (builtins/tests/lsof) at
// the unit level: a raw Name string containing ".." components that, left
// uncleaned, would look like a prefix match against the root, must be
// resolved by filepath.Clean before the containment check so it is
// correctly rejected.
func TestRedactNameDotDotTraversalNeutralized(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed"}}
	of := procfd.OpenFile{Name: "/allowed/../secret/passwd", IsPath: true}
	if got := redactName(of, roots, ""); got != "(restricted)" {
		t.Errorf("redactName = %q, want \"(restricted)\" (dot-dot traversal must not be treated as within /allowed)", got)
	}
}

// TestToRowBlanksDeviceSizeNodeOnRestrictedRow guards against leaking a
// restricted file's DEVICE/SIZE/NODE even though NAME is hidden: those are
// per-file attributes of the exact same out-of-sandbox path (an exact byte
// count, device number, and inode could otherwise still fingerprint a
// specific file such as /etc/shadow).
func TestToRowBlanksDeviceSizeNodeOnRestrictedRow(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed"}}
	of := procfd.OpenFile{
		Name:   "/other/secret",
		IsPath: true,
		Type:   "REG",
		Device: "8,1",
		Size:   "4096",
		Node:   "123456",
	}
	got := toRow(of, roots, "")
	if got.name != "(restricted)" {
		t.Errorf("name = %q, want \"(restricted)\"", got.name)
	}
	if got.device != "" || got.size != "" || got.node != "" {
		t.Errorf("device/size/node = %q/%q/%q, want all blank for a restricted row", got.device, got.size, got.node)
	}
}

// TestToRowKeepsDeviceSizeNodeWhenAllowed is the inverse of
// TestToRowBlanksDeviceSizeNodeOnRestrictedRow: a path within AllowedPaths
// must still show its real DEVICE/SIZE/NODE.
func TestToRowKeepsDeviceSizeNodeWhenAllowed(t *testing.T) {
	roots := []gateRoot{{raw: "/allowed"}}
	of := procfd.OpenFile{
		Name:   "/allowed/file",
		IsPath: true,
		Type:   "REG",
		Device: "8,1",
		Size:   "4096",
		Node:   "123456",
	}
	got := toRow(of, roots, "")
	if got.device != "8,1" || got.size != "4096" || got.node != "123456" {
		t.Errorf("device/size/node = %q/%q/%q, want unredacted values preserved", got.device, got.size, got.node)
	}
}

// TestToRowKeepsDeviceSizeNodeForNonPathTargets verifies that sockets/pipes,
// which are never gated (see procfd.OpenFile.IsPath), keep their
// DEVICE/SIZE/NODE regardless of AllowedPaths configuration.
func TestToRowKeepsDeviceSizeNodeForNonPathTargets(t *testing.T) {
	of := procfd.OpenFile{
		Name:   "socket:[12345]",
		IsPath: false,
		Type:   "sock",
		Device: "0,10",
		Node:   "98765",
	}
	got := toRow(of, nil, "")
	if got.device != "0,10" || got.node != "98765" {
		t.Errorf("device/node = %q/%q, want unredacted values preserved for a non-path target", got.device, got.node)
	}
}

// TestSelectorsMatchesORDefault verifies the default OR combination: with no
// selectors active everything matches, and with one or more active any
// single match is sufficient.
func TestSelectorsMatchesORDefault(t *testing.T) {
	of := procfd.OpenFile{PID: 100, Command: "alpha", UID: "1000"}

	if !(selectors{}).matches(of) {
		t.Error("no active selectors should match everything")
	}

	// Only -c matches; -p (a different PID) does not. OR should still match.
	sel := selectors{hasPIDs: true, pids: []int{999}, hasCmd: true, cmdPrefix: "al"}
	if !sel.matches(of) {
		t.Error("OR semantics: a match on -c alone should be sufficient")
	}

	// Neither selector matches.
	sel = selectors{hasPIDs: true, pids: []int{999}, hasCmd: true, cmdPrefix: "zz"}
	if sel.matches(of) {
		t.Error("OR semantics: no selector matching should not match")
	}
}

// TestSelectorsMatchesANDMode verifies -a's AND combination: every active
// selector must match, not just one.
func TestSelectorsMatchesANDMode(t *testing.T) {
	of := procfd.OpenFile{PID: 100, Command: "alpha", UID: "1000"}

	// -p matches, -c does not: AND must reject.
	sel := selectors{and: true, hasPIDs: true, pids: []int{100}, hasCmd: true, cmdPrefix: "zz"}
	if sel.matches(of) {
		t.Error("AND semantics: one non-matching selector should reject the entry")
	}

	// Both match: AND must accept.
	sel = selectors{and: true, hasPIDs: true, pids: []int{100}, hasCmd: true, cmdPrefix: "al"}
	if !sel.matches(of) {
		t.Error("AND semantics: all matching selectors should accept the entry")
	}
}

// TestParseUIDsCanonicalizesSpelling verifies that a UID accepted by
// strconv.Atoi in a non-canonical spelling (leading '+', leading zeros) is
// normalized to its plain decimal form. /proc/<pid>/status's Uid: field is
// always plain decimal, so an uncanonicalized "-u +1000" or "-u 01000" would
// otherwise never match any real process's UID column.
func TestParseUIDsCanonicalizesSpelling(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: "+1000", want: []string{"1000"}},
		{in: "01000", want: []string{"1000"}},
		{in: "0", want: []string{"0"}},
		{in: "1000,+1000,01000", want: []string{"1000"}},
	}
	for _, tt := range tests {
		got, err := parseUIDs(tt.in)
		if err != nil {
			t.Errorf("parseUIDs(%q) error = %v", tt.in, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseUIDs(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseUIDs(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

// TestParseUIDsRejectsNegative verifies that a negative UID, which
// strconv.Atoi accepts as a valid integer but which no real UID can ever
// be, is rejected rather than silently passed through as a selector that
// can never match.
func TestParseUIDsRejectsNegative(t *testing.T) {
	if _, err := parseUIDs("-1"); err == nil {
		t.Error("parseUIDs(\"-1\") should reject a negative UID")
	}
}

// TestProcessFilterMatchesSelectors verifies that selectors.processFilter
// produces the exact same accept/reject decision as selectors.matches
// evaluated against an OpenFile carrying the same PID/Command/UID — the
// property that makes it sound to reject a non-matching process before its
// fd directory is ever scanned.
func TestProcessFilterMatchesSelectors(t *testing.T) {
	sel := selectors{hasPIDs: true, pids: []int{100}, hasCmd: true, cmdPrefix: "al"}
	filter := sel.processFilter()
	if filter == nil {
		t.Fatal("processFilter returned nil with active selectors")
	}

	of := procfd.OpenFile{PID: 100, Command: "alpha", UID: "1000"}
	if got, want := filter(of.PID, of.Command, of.UID), sel.matches(of); got != want {
		t.Errorf("processFilter(%d, %q, %q) = %v, want %v (matches sel.matches)", of.PID, of.Command, of.UID, got, want)
	}

	of = procfd.OpenFile{PID: 999, Command: "zzz", UID: "1000"}
	if got, want := filter(of.PID, of.Command, of.UID), sel.matches(of); got != want {
		t.Errorf("processFilter(%d, %q, %q) = %v, want %v (matches sel.matches)", of.PID, of.Command, of.UID, got, want)
	}
}

// TestProcessFilterNilWithNoSelectors verifies that processFilter returns
// nil (matching every process) when no selector is active, matching
// selectors.matches' own unconditional-true behaviour in that case.
func TestProcessFilterNilWithNoSelectors(t *testing.T) {
	if got := (selectors{}).processFilter(); got != nil {
		t.Error("processFilter should be nil when no selector is active")
	}
}
