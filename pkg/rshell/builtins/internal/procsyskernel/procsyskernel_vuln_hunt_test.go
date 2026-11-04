// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procsyskernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntUnameProcReaderRejectsTraversalProcPath(t *testing.T) {
	paths := []string{
		"/proc/../tmp",
		"../proc",
		filepath.Join(t.TempDir(), "proc..backup"),
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			_, err := ReadFile(path, "ostype")
			if err == nil {
				t.Fatalf("expected unsafe procPath error")
			}
			if !strings.Contains(err.Error(), "unsafe procPath") {
				t.Fatalf("expected unsafe procPath error, got %v", err)
			}
			if strings.Contains(err.Error(), "open ") {
				t.Fatalf("unsafe procPath reached open path: %v", err)
			}
		})
	}
}

func TestVulnHuntUnameProcReaderRejectsPathLikeKernelNames(t *testing.T) {
	procPath := t.TempDir()
	names := []string{
		"../hostname",
		"sys/kernel/hostname",
		"host..name",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			_, err := ReadFile(procPath, name)
			if err == nil {
				t.Fatalf("expected unsafe name error")
			}
			if !strings.Contains(err.Error(), "unsafe name") {
				t.Fatalf("expected unsafe name error, got %v", err)
			}
			if strings.Contains(err.Error(), "open ") {
				t.Fatalf("unsafe name reached open path: %v", err)
			}
		})
	}
}

func TestVulnHuntUnameProcReaderCapsKernelFileReads(t *testing.T) {
	dir := t.TempDir()
	kernelDir := filepath.Join(dir, "sys", "kernel")
	if err := os.MkdirAll(kernelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := strings.Repeat("A", 8192)
	if err := os.WriteFile(filepath.Join(kernelDir, "hostname"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(dir, "hostname")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4096 {
		t.Fatalf("ReadFile returned %d bytes, want 4096", len(got))
	}
	if got != strings.Repeat("A", 4096) {
		t.Fatalf("ReadFile returned unexpected truncated content")
	}
}
