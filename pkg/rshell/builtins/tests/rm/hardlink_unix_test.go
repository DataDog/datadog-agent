// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// This is a Go test rather than a scenario test because the scenario framework
// has no way to create a hard link during fixture setup — the same reason
// AGENTS.md records for `free`, `ip route`, and `lsof`.
package rm_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestRemoveHardLinkUnlinksOnlyTheInSandboxName pins the deliberate asymmetry
// with the write primitives: `truncate`, `logrotate` and `>` refuse a multiply
// linked target because mutating it changes content visible under names
// outside the sandbox, but `rm` only removes one directory entry. The inode
// and its content survive under every other name, so unlinking is not a
// sandbox escape and is not gated. See the hard link entry in AGENTS.md.
func TestRemoveHardLinkUnlinksOnlyTheInSandboxName(t *testing.T) {
	base := t.TempDir()
	inside := filepath.Join(base, "rsh")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{inside, outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("SENSITIVE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(inside, "hard.txt")
	if err := os.Link(target, link); err != nil {
		t.Skipf("hard links unsupported on this filesystem: %v", err)
	}

	_, stderr, code := runScript(t, "rm "+link, inside,
		interp.AllowedPaths([]string{inside + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr %q)", code, stderr)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("expected in-sandbox name to be removed, got err %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("out-of-sandbox name should survive: %v", err)
	}
	if string(data) != "SENSITIVE\n" {
		t.Fatalf("out-of-sandbox content changed: %q", data)
	}
}
