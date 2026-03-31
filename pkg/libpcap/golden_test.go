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
// every corpus entry. The comparison against Go CompileBPFFilter will be
// enabled once the compiler pipeline is implemented in Phase 2-3.
func TestGoldenFiltertestOptimized(t *testing.T) {
	filtertestAvailable(t)
	filters := loadFilters(t)

	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			cOutput := runFiltertest(t, true, filter)
			if cOutput == "" {
				t.Fatal("C filtertest produced empty output")
			}
			t.Logf("C output (%d lines):\n%s", strings.Count(cOutput, "\n"), cOutput)
		})
	}
}

// TestGoldenFiltertestUnoptimized verifies unoptimized output.
func TestGoldenFiltertestUnoptimized(t *testing.T) {
	filtertestAvailable(t)
	filters := loadFilters(t)

	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			cOutput := runFiltertest(t, false, filter)
			if cOutput == "" {
				t.Fatal("C filtertest produced empty output")
			}
			t.Logf("C output (%d lines):\n%s", strings.Count(cOutput, "\n"), cOutput)
		})
	}
}
