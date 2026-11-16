// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package procnetsocket

import (
	"context"
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntSubsystemInvariantViolation_RejectsTraversalProcPath(t *testing.T) {
	ctx := context.Background()
	readers := []struct {
		name string
		fn   func(context.Context, string) ([]SocketEntry, error)
	}{
		{"tcp4", ReadTCP4},
		{"tcp6", ReadTCP6},
		{"udp4", ReadUDP4},
		{"udp6", ReadUDP6},
		{"unix", ReadUnix},
	}
	paths := []string{
		"/proc/../tmp",
		"../proc",
		"/tmp/proc..backup",
	}

	for _, reader := range readers {
		for _, path := range paths {
			t.Run(reader.name+"/"+path, func(t *testing.T) {
				_, err := reader.fn(ctx, path)
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
}

func TestVulnHuntSubsystemThreatModelCoverage_DefaultProcPathIsCanonicalProc(t *testing.T) {
	if DefaultProcPath != "/proc" {
		t.Fatalf("DefaultProcPath = %q, want /proc", DefaultProcPath)
	}
	clean, err := validateProcPath(DefaultProcPath)
	if err != nil {
		t.Fatalf("default proc path rejected: %v", err)
	}
	if clean != "/proc" {
		t.Fatalf("validateProcPath(%q) = %q, want /proc", DefaultProcPath, clean)
	}
}
