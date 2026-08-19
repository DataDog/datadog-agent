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

// Named sets are declared out of label order, one of them nests another, and the
// unrelated "default" group points at a set that was never posted: resolving it
// would be a bug.
const bepFixture = `
{"id":{"workspace":{}},"workspaceInfo":{"localExecRoot":"/exec"}}
{"id":{"namedSet":{"id":"1"}},"namedSetOfFiles":{"files":[{"name":"pkg/z/z_test_test.log.test2json.jsonl","uri":"file:///out/pkg/z/z_test_test.log.test2json.jsonl","pathPrefix":["bazel-out","cfg","bin"]}]}}
{"id":{"namedSet":{"id":"2"}},"namedSetOfFiles":{"fileSets":[{"id":"3"}]}}
{"id":{"namedSet":{"id":"3"}},"namedSetOfFiles":{"files":[{"name":"pkg/a/a_test_test.log.test2json.jsonl","uri":"bytestream://cache/blobs/abc/123","pathPrefix":["bazel-out","cfg","bin"]}]}}
{"id":{"targetCompleted":{"label":"//pkg/z:z_test"}},"completed":{"outputGroup":[{"name":"test2json","fileSets":[{"id":"1"}]}]}}
{"id":{"targetCompleted":{"label":"//pkg/a:a_test"}},"completed":{"outputGroup":[{"name":"default","fileSets":[{"id":"99"}]},{"name":"test2json","fileSets":[{"id":"2"}]}]}}
`

func writeBEP(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bep.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFragmentsFromBEP(t *testing.T) {
	got, err := fragmentsFromBEP(writeBEP(t, bepFixture))
	if err != nil {
		t.Fatal(err)
	}

	// Sorted by label, so //pkg/a comes first despite being posted last. Its
	// fragment is only in the remote cache, hence the exec-root fallback.
	want := []string{
		filepath.Join("/exec", "bazel-out", "cfg", "bin", "pkg/a/a_test_test.log.test2json.jsonl"),
		filepath.FromSlash("/out/pkg/z/z_test_test.log.test2json.jsonl"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fragments %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fragment %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFragmentsFromBEPRejectsUnknownFileSet(t *testing.T) {
	bep := `{"id":{"targetCompleted":{"label":"//pkg/a:a_test"}},"completed":{"outputGroup":[{"name":"test2json","fileSets":[{"id":"7"}]}]}}`

	_, err := fragmentsFromBEP(writeBEP(t, bep))
	if err == nil || !strings.Contains(err.Error(), `unknown fileSet "7"`) {
		t.Fatalf("expected unknown fileSet error, got %v", err)
	}
}

func TestFragmentsFromBEPWithoutAspectOutputs(t *testing.T) {
	bep := `{"id":{"targetCompleted":{"label":"//pkg/a:a_test"}},"completed":{"outputGroup":[{"name":"default","fileSets":[{"id":"1"}]}]}}`

	got, err := fragmentsFromBEP(writeBEP(t, bep))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %q, want no fragments", got)
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
	want := `{"Action":"pass"}` + "\n" + `{"Action":"fail"}` + "\n"
	if string(data) != want {
		t.Fatalf("got %q, want %q", data, want)
	}
}

func TestConcatFragmentsReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	err := concatFragments([]string{filepath.Join(dir, "absent.jsonl")}, filepath.Join(dir, "out.jsonl"))
	if err == nil || !strings.Contains(err.Error(), "open fragment") {
		t.Fatalf("expected open fragment error, got %v", err)
	}
}
