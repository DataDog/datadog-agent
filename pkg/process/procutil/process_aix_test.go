// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package procutil

import (
	"testing"
)

func makePsinfo(fname, psargs string) *psinfo {
	psi := &psinfo{}
	copy(psi.Fname[:], fname)
	copy(psi.Psargs[:], psargs)
	return psi
}

func TestPsinfoToProcessASCIIArgs(t *testing.T) {
	proc := psinfoToProcess(makePsinfo("rmcd", "/opt/rsct/bin/rmcd -a IBM.LPCommands"), 42)

	if proc.Name != "rmcd" || proc.Comm != "rmcd" {
		t.Errorf("expected name/comm rmcd, got %q/%q", proc.Name, proc.Comm)
	}
	want := []string{"/opt/rsct/bin/rmcd", "-a", "IBM.LPCommands"}
	if len(proc.Cmdline) != len(want) {
		t.Fatalf("expected cmdline %v, got %v", want, proc.Cmdline)
	}
	for i := range want {
		if proc.Cmdline[i] != want[i] {
			t.Errorf("expected cmdline[%d]=%q, got %q", i, want[i], proc.Cmdline[i])
		}
	}
}

func TestPsinfoToProcessNonASCIIArgs(t *testing.T) {
	// Readable argv can contain valid UTF-8 text; it must not be treated as
	// junk and replaced by the bracket fallback.
	proc := psinfoToProcess(makePsinfo("python", "python worker.py --label café"), 42)

	want := []string{"python", "worker.py", "--label", "café"}
	if len(proc.Cmdline) != len(want) {
		t.Fatalf("expected cmdline %v, got %v", want, proc.Cmdline)
	}
	for i := range want {
		if proc.Cmdline[i] != want[i] {
			t.Errorf("expected cmdline[%d]=%q, got %q", i, want[i], proc.Cmdline[i])
		}
	}
}

func TestPsinfoToProcessUnreadableArgs(t *testing.T) {
	// pr_psargs holds arbitrary binary bytes when the kernel cannot read a
	// process's argv; the cmdline must fall back to the bracketed name.
	junk := []byte{0x89, 0xa3, 0xe7, 'h', 'e', 'l', 'l', 'o', 0x1b, '[', 0x00}
	psi := makePsinfo("IBM.Softdird", "")
	copy(psi.Psargs[:], junk)

	proc := psinfoToProcess(psi, 42)

	if proc.Name != "IBM.Softdird" || proc.Comm != "IBM.Softdird" {
		t.Errorf("expected name/comm IBM.Softdird, got %q/%q", proc.Name, proc.Comm)
	}
	want := []string{"[IBM.Softdird]"}
	if len(proc.Cmdline) != 1 || proc.Cmdline[0] != want[0] {
		t.Errorf("expected cmdline %v, got %v", want, proc.Cmdline)
	}
}

func TestPsinfoToProcessEmptyNameAndArgs(t *testing.T) {
	// e.g. the swapper (pid 0): nothing to fall back to.
	proc := psinfoToProcess(makePsinfo("", ""), 0)

	if proc.Name != "" || proc.Comm != "" {
		t.Errorf("expected empty name/comm, got %q/%q", proc.Name, proc.Comm)
	}
	if len(proc.Cmdline) != 0 {
		t.Errorf("expected empty cmdline, got %v", proc.Cmdline)
	}
}
