// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package procsyskernel

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestVulnHuntUnameProcReaderRejectsFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	kernelDir := filepath.Join(dir, "sys", "kernel")
	if err := os.MkdirAll(kernelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(kernelDir, "hostname"), 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ReadFile(dir, "hostname")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected FIFO to be rejected")
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("expected regular-file rejection, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadFile blocked on FIFO")
	}
}
