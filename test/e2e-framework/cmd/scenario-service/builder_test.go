// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeBuilder returns a tiny shell script that echoes its args, standing in for
// a per-commit scenariorun binary.
type fakeBuilder struct{ bin string }

func (f fakeBuilder) Build(string) (string, error) { return f.bin, nil }

func writeStubBinary(t *testing.T) string {
	dir := t.TempDir()
	bin := filepath.Join(dir, "scenariorun")
	script := "#!/bin/sh\necho \"called: $*\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDriverShellsToBinary(t *testing.T) {
	bin := writeStubBinary(t)
	d := Driver{Builder: fakeBuilder{bin: bin}}
	out, err := d.Describe("abc123")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if want := "called: describe --json"; string(out) != "called: describe --json\n" {
		t.Fatalf("unexpected describe output: %q (want contains %q)", out, want)
	}
	// sanity: the stub binary is actually executable
	if _, err := exec.LookPath(bin); err != nil && !filepath.IsAbs(bin) {
		t.Fatalf("stub not executable: %v", err)
	}
}
