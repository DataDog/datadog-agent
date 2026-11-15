// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procnetroute

import (
	"context"
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntSubsystemInvariantViolation_RejectsTraversalProcPath(t *testing.T) {
	paths := []string{
		"/proc/../tmp",
		"../proc",
		"/tmp/proc..backup",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			_, err := ReadRoutes(context.Background(), path)
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

func TestVulnHuntSubsystemThreatModelCoverage_DefaultProcPathIsCanonicalProc(t *testing.T) {
	if DefaultProcPath != "/proc" {
		t.Fatalf("DefaultProcPath = %q, want /proc", DefaultProcPath)
	}
}
