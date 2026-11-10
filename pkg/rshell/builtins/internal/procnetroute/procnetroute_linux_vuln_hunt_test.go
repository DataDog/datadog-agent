// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procnetroute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProcNetRouteFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netDir, "route"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVulnHuntSubsystemResourceLimitBypass_RouteLineLengthCap(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		strings.Repeat("A", MaxLineBytes+1) + "\n"
	dir := writeProcNetRouteFile(t, content)

	_, err := ReadRoutes(context.Background(), dir)
	if err == nil {
		t.Fatalf("expected overlong route line to fail")
	}
	if !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("expected scanner token-too-long error, got %v", err)
	}
}

func TestVulnHuntSubsystemResourceLimitBypass_RouteMaxTotalLines(t *testing.T) {
	var b strings.Builder
	b.WriteString("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n")
	for range MaxTotalLines + 1 {
		b.WriteString("malformed\n")
	}
	dir := writeProcNetRouteFile(t, b.String())

	_, err := ReadRoutes(context.Background(), dir)
	if !errors.Is(err, ErrMaxTotalLines) {
		t.Fatalf("expected ErrMaxTotalLines, got %v", err)
	}
}

func TestVulnHuntSubsystemResourceLimitBypass_RouteMaxRoutes(t *testing.T) {
	const row = "eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	var b strings.Builder
	b.WriteString("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n")
	for range MaxRoutes + 1 {
		b.WriteString(row)
	}
	dir := writeProcNetRouteFile(t, b.String())

	_, err := ReadRoutes(context.Background(), dir)
	if !errors.Is(err, ErrMaxRoutes) {
		t.Fatalf("expected ErrMaxRoutes, got %v", err)
	}
}
