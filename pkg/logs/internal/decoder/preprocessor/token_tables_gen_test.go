// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTokenTablesUpToDate re-runs the generator and fails if token_tables_gen.go
// is stale (someone edited the master list in gen_token_tables.go without running
// `go generate`, or hand-edited the generated file).
func TestTokenTablesUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping generator run in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	// The output path must not end in ".go" — `go run` would treat it as another
	// source file to compile rather than a program argument.
	out := filepath.Join(t.TempDir(), "token_tables_gen.out")
	cmd := exec.Command(goBin, "run", "gen_token_tables.go", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running generator: %v\n%s", err, output)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("token_tables_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("token_tables_gen.go is out of date; run `go generate ./pkg/logs/internal/decoder/preprocessor`")
	}
}
