// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func binaryFixture(t *testing.T) Result {
	t.Helper()
	target := Target{runtime.GOOS, runtime.GOARCH}
	if target.Native() != nil {
		t.Skip("native ELF fixture")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := DescribeFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	tree := func() Tree {
		root := t.TempDir()
		path := filepath.Join(root, "file")
		if err := os.WriteFile(path, []byte("runtime fixture"), 0644); err != nil {
			t.Fatal(err)
		}
		f, err := DescribeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Path = "file"
		return Tree{Root: root, Files: []File{f}}
	}
	return Result{Schema: 1, Target: target, Provenance: Provenance{Producer: "invoke-binary"}, Binary: &BinaryBundle{Executable: file, Runtime: tree(), Assets: tree(), RuntimePrefix: "/source/repo/dev/embedded"}}
}
func TestRuntimeImageABIMismatchRejectedBeforeAgentProbe(t *testing.T) {
	r := binaryFixture(t)
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		if i.Args[0] == "rm" {
			return nil, nil
		}
		if i.Args[0] == "image" {
			return json.Marshal([]map[string]string{{"Id": "sha256:" + strings.Repeat("a", 64), "Os": r.Target.OS, "Architecture": r.Target.Arch}})
		}
		if !strings.Contains(strings.Join(i.Args, " "), "sysconfig") {
			t.Fatalf("Agent started despite ABI mismatch: %+v", i)
		}
		return []byte(`{"abi":"python3.13","path":"/opt/datadog-agent/embedded/lib/python3.13/site-packages","os":"ubuntu","version":"24.04"}`), nil
	}}
	if _, err := a.VerifyRuntime(context.Background(), r, "localhost/runtime:7.83.0"); err == nil || !strings.Contains(err.Error(), "ABI mismatch") {
		t.Fatal(err)
	}
}
func TestBinaryGenerationsOwnFullRuntimeInventory(t *testing.T) {
	r := binaryFixture(t)
	first, err := Stage(r, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Stage(r, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if first.Binary.Runtime.Root == second.Binary.Runtime.Root {
		t.Fatal("shared mutable runtime")
	}
	if err := os.WriteFile(filepath.Join(r.Binary.Runtime.Root, "file"), []byte("changed source runtime"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := first.Validate(r.Target); err != nil {
		t.Fatal(err)
	}
	if err := second.Validate(r.Target); err != nil {
		t.Fatal(err)
	}
}
