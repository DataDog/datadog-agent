// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stackbudget

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// budget is the kernel's own limit: a program over it fails to load. The
// dyninst program has historically sat right at it on arm64-debug, so we check
// it here rather than waiting for a KMT job to catch it.
const budget = MaxStack

// objects maps a variant name to the env var the build rule uses to hand over
// that variant's compiled object. Both architectures are checked from whichever
// host runs the test, since frame sizes are compiler output.
var objects = map[string]string{
	"arm64":        "DYNINST_EBPF_OBJ_ARM64",
	"arm64-debug":  "DYNINST_EBPF_OBJ_ARM64_DEBUG",
	"x86_64":       "DYNINST_EBPF_OBJ_X86_64",
	"x86_64-debug": "DYNINST_EBPF_OBJ_X86_64_DEBUG",
}

func TestCombinedStackBudget(t *testing.T) {
	checked := 0
	for variant, envVar := range objects {
		path := os.Getenv(envVar)
		if path == "" {
			continue
		}
		checked++
		t.Run(variant, func(t *testing.T) {
			resolved, err := locate(path)
			if err != nil {
				t.Fatalf("locating %s: %v", path, err)
			}
			report, err := Analyze(resolved)
			if err != nil {
				t.Fatalf("analyzing %s: %v", resolved, err)
			}
			for entry, chain := range report.Worst {
				t.Logf("%s: %s: worst chain %s (headroom %d bytes)",
					variant, entry, chain, budget-chain.Total)
			}
			worst := report.WorstChain()
			if worst.Total > budget {
				t.Errorf("combined stack of %d bytes exceeds the %d-byte budget; "+
					"the verifier will refuse to load this program.\nworst chain: %s\n"+
					"Shrink a frame on this chain to fix it.",
					worst.Total, budget, worst)
			}
		})
	}
	if checked == 0 {
		t.Skip("no eBPF objects provided; run via Bazel so the cross-architecture " +
			"objects are built and passed through the environment")
	}
}

// locate resolves a data dependency's path: absolute as-is, otherwise try the
// Bazel runfiles tree before walking up from the working directory.
func locate(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	candidates := []string{path}
	if dir := os.Getenv("TEST_SRCDIR"); dir != "" {
		if ws := os.Getenv("TEST_WORKSPACE"); ws != "" {
			candidates = append(candidates, filepath.Join(dir, ws, path))
		}
		candidates = append(candidates, filepath.Join(dir, path))
	}
	// Fall back to walking up, which covers running the test directly.
	prefix := ""
	for i := 0; i < 8; i++ {
		prefix = filepath.Join(prefix, "..")
		candidates = append(candidates, filepath.Join(prefix, path))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("not found in any of %v", candidates)
}
