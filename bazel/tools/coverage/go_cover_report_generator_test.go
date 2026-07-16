package main

import (
	"os"
	"path/filepath"
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
	want := `mode: atomic
example.com/bar.go:1.1,1.5 1 1
example.com/foo.go:1.1,1.5 1 5
example.com/foo.go:2.1,2.5 1 0
`
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
	if string(got) != "mode: atomic\n" {
		t.Fatalf("unexpected baseline profile: %q", got)
	}
}
