// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"os"
	"testing"
)

func TestCheckSymbolsReportsViolations(t *testing.T) {
	failures, err := checkSymbols("foo\nbar_Cfunc_x\n", []string{`^missing$`}, []string{`_Cfunc_`})
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 {
		t.Fatalf("want 2 failures, got %v", failures)
	}
}

func TestReadSymbolsRejectsNonBinary(t *testing.T) {
	path := t.TempDir() + "/not_a_binary"
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSymbols(path); err == nil {
		t.Fatal("expected an error for a non-binary file")
	}
}
