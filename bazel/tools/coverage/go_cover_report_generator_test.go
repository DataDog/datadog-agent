// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateReport(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.out")
	second := filepath.Join(dir, "second.out")
	baseline := filepath.Join(dir, "baseline.dat")
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	writeTestFile(t, first, `mode: atomic
example.com/foo.go:1.1,1.5 1 2
example.com/foo.go:2.1,2.5 1 0
`)
	writeTestFile(t, second, `mode: atomic
example.com/bar.go:1.1,1.5 1 1
example.com/foo.go:1.1,1.5 1 3
`)
	writeTestFile(t, baseline, `SF:example.com/foo.go
end_of_record
`)
	writeTestFile(t, reports, first+"\n"+baseline+"\n"+second+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	// foo.go:1.1 is covered by both inputs (2 and 3): unioned to 1, not summed to 5.
	want := `mode: set
example.com/bar.go:1.1,1.5 1 1
example.com/foo.go:1.1,1.5 1 1
example.com/foo.go:2.1,2.5 1 0
`
	if string(got) != want {
		t.Fatalf("unexpected merged profile:\n%s", got)
	}
}

// TestGenerateReportUnionsLargeCounts guards the invariant behind unionCount: no
// merged block ever exceeds 1, however many targets covered it or how hot it was.
// Summing here is what drove a real block to 3x10^8 executions in CI.
func TestGenerateReportUnionsLargeCounts(t *testing.T) {
	dir := t.TempDir()
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	var paths []string
	for i, count := range []string{"1000000", "2000000", "3000000"} {
		path := filepath.Join(dir, "profile"+string(rune('a'+i))+".out")
		writeTestFile(t, path, "mode: atomic\nexample.com/hot.go:1.1,1.5 1 "+count+"\n")
		paths = append(paths, path)
	}
	writeTestFile(t, reports, strings.Join(paths, "\n")+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := "mode: set\nexample.com/hot.go:1.1,1.5 1 1\n"
	if string(got) != want {
		t.Fatalf("unexpected merged profile:\n%s", got)
	}
}

func TestGenerateBaselineReport(t *testing.T) {
	dir := t.TempDir()
	baseline := filepath.Join(dir, "baseline.dat")
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	writeTestFile(t, baseline, "SF:example.com/foo.go\nend_of_record\n")
	writeTestFile(t, reports, baseline+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := `mode: set
example.com/foo.go:1.0,1.1 1 0
`
	if string(got) != want {
		t.Fatalf("unexpected baseline profile:\n%s", got)
	}
}

func TestGenerateBaselineFromSourceFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "uncovered.go")
	baseline := filepath.Join(dir, "baseline.dat")
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	writeTestFile(t, source, "package main\n\nfunc uncovered() {}\n")
	writeTestFile(t, baseline, "SF:"+source+"\nend_of_record\n")
	writeTestFile(t, reports, baseline+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := `mode: set
` + source + `:1.0,1.1 1 0
` + source + `:3.0,3.1 1 0
`
	if string(got) != want {
		t.Fatalf("unexpected baseline profile:\n%s", got)
	}
}

func TestGenerateReportWithUncoveredBaselineFile(t *testing.T) {
	dir := t.TempDir()
	covered := filepath.Join(dir, "covered.out")
	baseline := filepath.Join(dir, "baseline.dat")
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	writeTestFile(t, covered, `mode: atomic
example.com/covered.go:1.1,1.5 1 1
`)
	writeTestFile(t, baseline, `SF:example.com/uncovered.go
end_of_record
`)
	writeTestFile(t, reports, covered+"\n"+baseline+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := `mode: set
example.com/covered.go:1.1,1.5 1 1
example.com/uncovered.go:1.0,1.1 1 0
`
	if string(got) != want {
		t.Fatalf("unexpected merged profile:\n%s", got)
	}
}

func TestGenerateCoverageDirReport(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "coverage")
	if err := os.Mkdir(coverageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(coverageDir, "first.dat")
	second := filepath.Join(coverageDir, "second.dat")
	output := filepath.Join(dir, "merged.out")
	manifest := filepath.Join(dir, "manifest.txt")

	writeTestFile(t, first, `mode: atomic
example.com/foo.go:1.1,1.5 1 2
`)
	writeTestFile(t, second, `mode: atomic
example.com/bar.go:1.1,1.5 1 1
/usr/lib/foo.go:1.1,1.5 1 1
`)
	writeTestFile(t, manifest, "example.com/foo.go\nexample.com/bar.go\n")

	if err := generateCoverageDirReport(
		coverageDir,
		output,
		[]string{"/usr/lib/.+"},
		manifest,
		"",
	); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := `mode: set
example.com/bar.go:1.1,1.5 1 1
example.com/foo.go:1.1,1.5 1 1
`
	if string(got) != want {
		t.Fatalf("unexpected merged profile:\n%s", got)
	}
}

func TestIgnoresFullLcovProfile(t *testing.T) {
	dir := t.TempDir()
	lcov := filepath.Join(dir, "coverage.dat")
	reports := filepath.Join(dir, "reports.txt")
	output := filepath.Join(dir, "merged.out")

	writeTestFile(t, lcov, `SF:example.com/foo.go
DA:1,2
DA:2,0
LH:1
LF:2
end_of_record
`)
	writeTestFile(t, reports, lcov+"\n")

	if err := generateReport(reports, output); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "mode: set\n" {
		t.Fatalf("unexpected merged profile:\n%s", got)
	}
}
