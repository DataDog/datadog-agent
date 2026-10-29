// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// These are Go tests rather than scenario tests because the scenario framework
// has no way to create a hard link during fixture setup — the same reason
// AGENTS.md records for `free`, `ip route`, and `lsof`. The behaviour under
// test is also an intentional divergence from bash, which happily truncates a
// hard link, so it could not be asserted against bash either. The guard is
// unix-only because Windows cannot report a link count from an open handle
// without a new syscall surface (see allowedpaths/hardlink_windows.go).
package truncate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestTruncateRejectsHardLinkToFileOutsideSandbox is the regression test for
// the path-based-containment gap: AllowedPaths resolves paths, but a hard link
// inside a :rw root is a second name for an inode that may also be named
// outside every configured root. Truncating it would destroy out-of-sandbox
// content while every path check passes.
func TestTruncateRejectsHardLinkToFileOutsideSandbox(t *testing.T) {
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

	_, stderr, code := runScript(t, "truncate -s 0 "+link, inside,
		interp.AllowedPaths([]string{inside + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit, got 0 (stderr %q)", stderr)
	}
	if !strings.Contains(stderr, "hard links are not supported as write targets") {
		t.Fatalf("expected hard-link rejection in stderr, got %q", stderr)
	}
	if got := fileSize(t, target); got != int64(len("SENSITIVE\n")) {
		t.Fatalf("out-of-sandbox content was mutated: size %d", got)
	}
}

// TestTruncateHardLinkGuardAllowsSingleLinkedFile pins the happy path so the
// guard cannot silently break ordinary truncation.
func TestTruncateHardLinkGuardAllowsSingleLinkedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "plain.txt", "hello\n")

	_, stderr, code := truncateRun(t, "truncate -s 0 "+path, dir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr %q)", code, stderr)
	}
	if got := fileSize(t, path); got != 0 {
		t.Fatalf("expected truncated file, size %d", got)
	}
}

// TestTruncateRejectsHardLinkEntirelyInsideSandbox documents that the guard is
// deliberately coarse: it rejects any multiply linked regular file, including
// one whose other names all live inside the sandbox. rshell cannot enumerate
// an inode's names, so it cannot tell the two cases apart and fails closed.
func TestTruncateRejectsHardLinkEntirelyInsideSandbox(t *testing.T) {
	dir := t.TempDir()
	original := writeFile(t, dir, "a.txt", "data\n")
	link := filepath.Join(dir, "b.txt")
	if err := os.Link(original, link); err != nil {
		t.Skipf("hard links unsupported on this filesystem: %v", err)
	}

	_, stderr, code := truncateRun(t, "truncate -s 0 "+link, dir)
	if code == 0 {
		t.Fatalf("expected non-zero exit, got 0 (stderr %q)", stderr)
	}
	if !strings.Contains(stderr, "hard links are not supported as write targets") {
		t.Fatalf("expected hard-link rejection in stderr, got %q", stderr)
	}
}

// TestRedirectRejectsHardLinkToFileOutsideSandbox covers the `>` / `>>`
// redirection primitive, which reaches the same guard through Sandbox.Open.
func TestRedirectRejectsHardLinkToFileOutsideSandbox(t *testing.T) {
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

	for _, op := range []string{">", ">>"} {
		_, stderr, code := runScript(t, "echo pwned "+op+" "+link, inside,
			interp.AllowedPaths([]string{inside + ":rw"}),
			interp.WithMode(interp.ModeRemediation),
		)
		if code == 0 {
			t.Fatalf("%s: expected non-zero exit, got 0 (stderr %q)", op, stderr)
		}
		if !strings.Contains(stderr, "hard links are not supported as write targets") {
			t.Fatalf("%s: expected hard-link rejection in stderr, got %q", op, stderr)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "SENSITIVE\n" {
			t.Fatalf("%s: out-of-sandbox content was mutated: %q", op, data)
		}
	}
}

// TestRedirectHardLinkGuardAllowsSingleLinkedFile pins the redirection happy
// path.
func TestRedirectHardLinkGuardAllowsSingleLinkedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	_, stderr, code := truncateRun(t, "echo ok > "+path, dir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr %q)", code, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("unexpected content %q", data)
	}
}
