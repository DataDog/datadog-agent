// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func packageFixture(t *testing.T) Result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.deb")
	if err := os.WriteFile(path, []byte("test package bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := DescribeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return Result{Schema: 1, Target: Target{"linux", "amd64"}, Provenance: Provenance{Producer: "existing-package"}, Package: &Package{File: file, Name: "datadog-agent", Format: "deb", Version: "7.83.0", Roles: []string{"agent"}}}
}
func TestManifestRejectsCorruptionAndWrongFormat(t *testing.T) {
	r := packageFixture(t)
	if err := r.Validate(r.Target); err != nil {
		t.Fatal(err)
	}
	r.Binary = &BinaryBundle{}
	if r.Validate(r.Target) == nil {
		t.Fatal("accepted multiple formats")
	}
	r.Binary = nil
	if r.Validate(Target{"linux", "arm64"}) == nil {
		t.Fatal("accepted wrong arch")
	}
	if err := os.WriteFile(r.Package.File.Path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if r.Validate(r.Target) == nil {
		t.Fatal("accepted changed file")
	}
}
func TestStrictManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	for _, content := range []string{`{"schema":1,"unknown":true}`, `{"schema":1} {}`} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path); err == nil {
			t.Fatal("accepted malformed receipt")
		}
	}
}
func TestRuntimeInventoryRejectsEscapeAndUnlistedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lib.so")
	os.WriteFile(path, []byte("library"), 0644)
	file, _ := DescribeFile(path)
	file.Path = "lib.so"
	tree := Tree{Root: root, Files: []File{file}}
	if err := verifyTree(tree); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "extra"), []byte("unexpected"), 0600)
	if verifyTree(tree) == nil {
		t.Fatal("accepted undeclared dependency")
	}
	os.Remove(filepath.Join(root, "extra"))
	outside := filepath.Join(t.TempDir(), "escape")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(root, "link"))
	tree.Files = append(tree.Files, File{Path: "link", Link: outside})
	if verifyTree(tree) == nil {
		t.Fatal("accepted symlink escape")
	}
	tree.Files = []File{{Path: "../outside"}}
	if verifyTree(tree) == nil {
		t.Fatal("accepted traversal")
	}
}
func TestStageGenerationsAreIndependent(t *testing.T) {
	r := packageFixture(t)
	dir := t.TempDir()
	first, err := Stage(r, dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Stage(r, dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Package.File.Path == second.Package.File.Path {
		t.Fatal("generation overwritten")
	}
	os.WriteFile(r.Package.File.Path, []byte("source changed"), 0600)
	if err = first.Validate(first.Target); err != nil {
		t.Fatal(err)
	}
	if err = second.Validate(second.Target); err != nil {
		t.Fatal(err)
	}
}
func TestExistingImageNeverBuildsAndVerifiesReceipt(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	calls := []Invocation{}
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		calls = append(calls, i)
		return []byte(`[{"Id":"` + id + `","Os":"linux","Architecture":"amd64"}]`), nil
	}}
	r, err := a.ExistingImage(context.Background(), "localhost/agent:dev", "", Target{"linux", "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "result.json")
	if err = Write(manifest, r); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ExistingImage(context.Background(), "localhost/agent:dev", manifest, r.Target); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if call.Program != "docker" || !reflect.DeepEqual(call.Args, []string{"image", "inspect", "localhost/agent:dev"}) {
			t.Fatalf("existing acquisition built or mutated: %+v", call)
		}
	}
	r.Image.ID = "sha256:" + strings.Repeat("b", 64)
	Write(manifest, r)
	if _, err = a.ExistingImage(context.Background(), "localhost/agent:dev", manifest, r.Target); err == nil {
		t.Fatal("accepted mismatched receipt")
	}
}
func TestDeliverableTagUsesActualIdentity(t *testing.T) {
	id := "sha256:" + strings.Repeat("c", 64)
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		if i.Args[0] == "tag" {
			if i.Args[1] != id || !strings.Contains(i.Args[2], "7.99.0-e2ectl."+strings.Repeat("c", 64)) {
				t.Fatalf("bad tag command: %v", i.Args)
			}
			return nil, nil
		}
		return json.Marshal([]map[string]string{{"Id": id, "Os": "linux", "Architecture": "amd64"}})
	}}
	r := Result{Schema: 1, Target: Target{"linux", "amd64"}, Provenance: Provenance{Producer: "existing-image"}, Image: &Image{Reference: "localhost/agent:mutable", ID: id}}
	out, err := a.DeliverableImage(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Image.Delivered == r.Image.Reference {
		t.Fatal("mutable tag retained")
	}
	if r.Image.Delivered != "" {
		t.Fatal("input receipt mutated")
	}
}
func TestExistingPackageChecksMetadataWithoutBuild(t *testing.T) {
	r := packageFixture(t)
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		if i.Program != "dpkg-deb" {
			t.Fatalf("unexpected build command: %+v", i)
		}
		if i.Args[0] == "--contents" {
			return []byte("-rwxr-xr-x root/root 10 2026-01-01 00:00 ./opt/datadog-agent/bin/agent/agent"), nil
		}
		if i.Args[0] == "-f" {
			return []byte("libc6"), nil
		}
		return []byte("datadog-agent\n7.83.0\namd64\n"), nil
	}}
	if _, err := a.ExistingPackage(context.Background(), r.Package.File.Path, "", r.Target); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExistingPackage(context.Background(), r.Package.File.Path, "", Target{"linux", "arm64"}); err == nil {
		t.Fatal("accepted package target mismatch")
	}
}
func TestInvokeBuildFailureDoesNotPublishOrStage(t *testing.T) {
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, "tasks"), 0700)
	os.WriteFile(filepath.Join(repo, "tasks", "agent.py"), nil, 0600)
	output := t.TempDir()
	target := Target{runtime.GOOS, runtime.GOARCH}
	if target.Native() != nil {
		t.Skip("native Linux adapter")
	}
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		if !i.StreamOutput || i.Program != "dda" || i.Args[1] != "agent.build" || !strings.HasPrefix(i.Args[3], "--result-manifest=") {
			t.Fatalf("unsanctioned build: %+v", i)
		}
		return nil, errors.New("build failed")
	}}
	if _, err := a.BuildBinary(context.Background(), BinaryRequest{Request: Request{Repository: repo, OutputDir: output, Target: target}}); err == nil {
		t.Fatal("build unexpectedly passed")
	}
	entries, _ := os.ReadDir(output)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "generation-") {
			t.Fatal("published failed preparation")
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".e2ectl-artifact-build.lock")); !os.IsNotExist(err) {
		t.Fatal("build lock not released")
	}
}
func TestCrossBuildRejectedBeforeExecutor(t *testing.T) {
	a := Adapter{Run: func(context.Context, Invocation) ([]byte, error) { t.Fatal("cross-build command ran"); return nil, nil }}
	_, err := a.BuildBinary(context.Background(), BinaryRequest{Request: Request{Target: Target{"windows", "amd64"}}})
	if err == nil {
		t.Fatal("cross build accepted")
	}
}
