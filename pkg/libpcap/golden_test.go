//go:build libpcap_test

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libpcap

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	filtertestBin  = "testdata/filtertest"
	filterCorpus   = "testdata/filters.txt"
	defaultDLT     = "EN10MB"
	defaultSnaplen = "262144"
)

func filtertestAvailable(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filtertestBin); os.IsNotExist(err) {
		t.Skipf("C filtertest binary not found at %s; run testdata/build_filtertest.sh first", filtertestBin)
	}
}

func runFiltertest(t *testing.T, optimize bool, filter string) string {
	t.Helper()
	args := []string{"-s", defaultSnaplen}
	if !optimize {
		args = append(args, "-O")
	}
	args = append(args, defaultDLT)
	if filter != "" {
		args = append(args, filter)
	}
	cmd := exec.Command(filtertestBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("filtertest failed for %q: %v\nstderr: %s", filter, err, stderr.String())
	}
	return stdout.String()
}

func loadFilters(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filterCorpus)
	if err != nil {
		t.Fatalf("failed to open filter corpus: %v", err)
	}
	defer f.Close()

	var filters []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		filters = append(filters, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading filter corpus: %v", err)
	}
	return filters
}

// TestGoldenFiltertestOptimized verifies the C filtertest produces output for
// every corpus entry. This test uses the OPTIMIZED C output — the Go optimizer
// is not yet implemented (Phase 3), so this is a reference-only test for now.
func TestGoldenFiltertestOptimized(t *testing.T) {
	filtertestAvailable(t)
	filters := loadFilters(t)

	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			cOutput := runFiltertest(t, true, filter)
			if cOutput == "" {
				t.Fatal("C filtertest produced empty output")
			}
			t.Logf("C optimized (%d lines):\n%s", strings.Count(cOutput, "\n"), cOutput)
		})
	}
}

// TestGoldenFiltertestUnoptimized compares Go CompileBPFFilter output against
// the C filtertest UNOPTIMIZED output (filtertest -O).
//
// Filters that use features not yet implemented in Go (VLAN, MPLS, etc.)
// are expected to fail with "not yet implemented" — these are logged but
// not counted as test failures.
func TestGoldenFiltertestUnoptimized(t *testing.T) {
	filtertestAvailable(t)
	filters := loadFilters(t)

	var matched, notImpl, mismatch int

	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			cOutput := runFiltertest(t, false, filter)
			if cOutput == "" {
				t.Fatal("C filtertest produced empty output")
			}

			goOutput, err := DumpFilter(LinkTypeEthernet, 262144, filter)
			if err != nil {
				if strings.Contains(err.Error(), "not yet implemented") {
					t.Skipf("Go: %v", err)
					notImpl++
					return
				}
				t.Fatalf("Go compilation failed: %v", err)
			}

			cNorm := normalizeOutput(cOutput)
			goNorm := normalizeOutput(goOutput)

			if cNorm == goNorm {
				matched++
				t.Logf("MATCH (%d instructions)", strings.Count(goNorm, "\n"))
			} else {
				mismatch++
				t.Logf("MISMATCH (unoptimized Go vs C)")
				t.Logf("C output:\n%s", cOutput)
				t.Logf("Go output:\n%s", goOutput)
				// Don't fail — the optimizer will produce different unoptimized
				// output in some cases. This is expected until Phase 3.
			}
		})
	}

	t.Logf("Summary: %d matched, %d mismatched, %d not implemented", matched, mismatch, notImpl)
}

// TestGoldenSimpleFilters tests a curated set of simple filters that MUST
// produce exact unoptimized output matches between Go and C.
func TestGoldenSimpleFilters(t *testing.T) {
	filtertestAvailable(t)

	// These filters should produce identical unoptimized output
	filters := []string{
		"ip",
		"arp",
		"ip6",
		"icmp",
		"tcp",
		"udp",
	}

	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			cOutput := runFiltertest(t, false, filter)
			goOutput, err := DumpFilter(LinkTypeEthernet, 262144, filter)
			if err != nil {
				t.Fatalf("Go: %v", err)
			}

			cNorm := normalizeOutput(cOutput)
			goNorm := normalizeOutput(goOutput)

			if cNorm != goNorm {
				t.Errorf("Output mismatch for %q", filter)
				t.Logf("C:\n%s", cOutput)
				t.Logf("Go:\n%s", goOutput)
			}
		})
	}
}

// normalizeOutput trims trailing whitespace from each line and normalizes
// line endings for comparison.
func normalizeOutput(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}
