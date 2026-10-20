// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestNewAnalyzer_Metadata verifies that the returned Analyzer has the correct
// name and does not declare unnecessary framework dependencies.
func TestNewAnalyzer_Metadata(t *testing.T) {
	a := NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"fmt.Println"},
		ListName: "test",
	})
	if a.Name != "analysis" {
		t.Errorf("Name = %q, want \"analysis\"", a.Name)
	}
	if len(a.Requires) != 0 {
		t.Errorf("Requires has %d entries, want 0", len(a.Requires))
	}
}

// TestNewAnalyzer_MalformedEntry verifies that NewAnalyzer panics immediately
// when given an allowlist entry with no dot separator, matching the test-harness
// behaviour (t.Fatalf on the same condition).
func TestNewAnalyzer_MalformedEntry(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for malformed allowlist entry, got none")
		}
	}()
	NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"nodot"},
		ListName: "test",
	})
}

// TestNewAnalyzer_AllowedImport runs the analyzer against a package that uses
// only allowlisted symbols; no diagnostics should be emitted.
func TestNewAnalyzer_AllowedImport(t *testing.T) {
	testdata := analysistest.TestData()
	a := NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"fmt.Println"},
		ListName: "test",
	})
	analysistest.Run(t, testdata, a, "good")
}

// TestNewAnalyzer_BannedImport verifies that a permanently banned import is
// reported at the import site.
func TestNewAnalyzer_BannedImport(t *testing.T) {
	testdata := analysistest.TestData()
	a := NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"fmt.Println"},
		ListName: "test",
	})
	analysistest.Run(t, testdata, a, "bannedimport")
}

// TestNewAnalyzer_DisallowedImport verifies that an import not in the
// allowlist is reported at the import site.
func TestNewAnalyzer_DisallowedImport(t *testing.T) {
	testdata := analysistest.TestData()
	a := NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"fmt.Println"},
		ListName: "test",
	})
	analysistest.Run(t, testdata, a, "disallowedimport")
}

// TestNewAnalyzer_UnusedSymbol verifies that an allowlisted symbol that is
// never referenced in the package is reported at the package declaration.
func TestNewAnalyzer_UnusedSymbol(t *testing.T) {
	testdata := analysistest.TestData()
	a := NewAnalyzer(AnalyzerConfig{
		Symbols:  []string{"fmt.Println", "fmt.Sprintf"},
		ListName: "test",
	})
	analysistest.Run(t, testdata, a, "unusedsymbol")
}
