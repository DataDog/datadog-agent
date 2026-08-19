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

func TestFragmentBasename(t *testing.T) {
	logPath := "/exec/bazel-out/k8-fastbuild/testlogs/pkg/foo/foo_test/test.log"
	got := fragmentBasename("foo_test", logPath)
	want := "foo_test_test.log.test2json.jsonl"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestConcatFragments(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.jsonl")
	second := filepath.Join(dir, "b.jsonl")
	if err := os.WriteFile(first, []byte(`{"Action":"pass"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"Action":"fail"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.jsonl")
	if err := concatFragments([]string{first, second}, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"Action":"pass"`) || !strings.Contains(string(data), `"Action":"fail"`) {
		t.Fatalf("unexpected merged output: %s", data)
	}
}
