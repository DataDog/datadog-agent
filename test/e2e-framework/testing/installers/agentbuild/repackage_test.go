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
	"runtime"
	"strings"
	"testing"
)

func TestRepackageRejectsInstalledHostMounts(t *testing.T) {
	for _, path := range []string{"/", "/opt", "/opt/datadog-agent", "/opt/datadog-agent/checkout", "/tmp/repo,src=/opt"} {
		if safeRepackageMount(path) == nil {
			t.Fatalf("unsafe host mount accepted: %s", path)
		}
	}
}

func TestRepackageUsesExistingTaskOnlyInsideIsolatedContainer(t *testing.T) {
	target := Target{runtime.GOOS, runtime.GOARCH}
	if target.Native() != nil {
		t.Skip("native Linux adapter")
	}
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, "tasks"), 0700)
	os.WriteFile(filepath.Join(repo, "tasks", "agent.py"), nil, 0600)
	called := false
	a := Adapter{Run: func(_ context.Context, i Invocation) ([]byte, error) {
		if i.Program == "git" {
			return []byte(filepath.Join(repo, ".git")), nil
		}
		if i.Program != "docker" {
			t.Fatalf("host task execution: %+v", i)
		}
		if i.Args[0] == "image" {
			return json.Marshal([]map[string]string{{"Id": "sha256:" + strings.Repeat("a", 64), "Os": target.OS, "Architecture": target.Arch}})
		}
		if i.Args[0] == "rm" {
			return nil, nil
		}
		if !i.StreamOutput {
			t.Fatal("Omnibus safety prompt would be hidden")
		}
		called = true
		cmd := strings.Join(i.Args, " ")
		for _, forbidden := range []string{"--privileged", "docker.sock", "src=/opt", "--yes"} {
			if strings.Contains(cmd, forbidden) {
				t.Fatalf("unsafe repack: %s", cmd)
			}
		}
		for _, required := range []string{"--entrypoint dda", "inv omnibus.build-repackaged-agent", "--base-package-url=https://example.com/agent.deb", "--base-package-sha256=", "--result-manifest=", "--pull=never"} {
			if !strings.Contains(cmd, required) {
				t.Fatalf("missing contract %s: %s", required, cmd)
			}
		}
		return nil, errors.New("offline command test: do not build")
	}}
	_, err := a.Repackage(context.Background(), RepackageRequest{Request: Request{Repository: repo, OutputDir: t.TempDir(), Target: target}, BuildImage: "localhost/build:native", BasePackageURL: "https://example.com/agent.deb", BasePackageSHA256: strings.Repeat("a", 64)})
	if err == nil || !called {
		t.Fatalf("repack command not exercised: %v", err)
	}
}
