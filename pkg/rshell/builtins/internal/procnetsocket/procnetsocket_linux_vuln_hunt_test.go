// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procnetsocket

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProcNetSocketFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVulnHuntSubsystemResourceLimitBypass_SocketLineLengthCap(t *testing.T) {
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		strings.Repeat("A", MaxLineBytes+1) + "\n"
	dir := writeProcNetSocketFile(t, "tcp", content)

	_, err := ReadTCP4(context.Background(), dir)
	if err == nil {
		t.Fatalf("expected overlong socket line to fail")
	}
	if !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("expected scanner token-too-long error, got %v", err)
	}
}

func TestVulnHuntSubsystemResourceLimitBypass_SocketMaxTotalLines(t *testing.T) {
	var b strings.Builder
	b.WriteString("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	for range MaxTotalLines + 1 {
		b.WriteString("malformed\n")
	}
	dir := writeProcNetSocketFile(t, "tcp", b.String())

	_, err := ReadTCP4(context.Background(), dir)
	if !errors.Is(err, ErrMaxTotalLines) {
		t.Fatalf("expected ErrMaxTotalLines, got %v", err)
	}
}

func TestVulnHuntSubsystemResourceLimitBypass_SocketMaxEntries(t *testing.T) {
	const row = "0: 0100007F:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 12345 1 0000000000000000 100 0 0 10 0\n"
	var b strings.Builder
	b.WriteString("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	for range MaxEntries + 1 {
		b.WriteString(row)
	}
	dir := writeProcNetSocketFile(t, "tcp", b.String())

	_, err := ReadTCP4(context.Background(), dir)
	if !errors.Is(err, ErrMaxEntries) {
		t.Fatalf("expected ErrMaxEntries, got %v", err)
	}
}
