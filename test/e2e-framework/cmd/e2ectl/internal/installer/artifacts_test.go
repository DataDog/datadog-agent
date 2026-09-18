// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

func TestHelmBuildSelectionReplacesLegacySourceFields(t *testing.T) {
	cfg := &config.File{Agent: config.Agent{Install: "helm"}}
	if len((&Kubernetes{}).Validate(cfg)) == 0 {
		t.Fatal("missing legacy source accepted")
	}
	cfg.Agent.Build = &config.BuildSelection{Provider: "existing-image", Section: []byte("reference: localhost/agent:dev")}
	if errs := (&Kubernetes{}).Validate(cfg); len(errs) > 0 {
		t.Fatal(errs)
	}
	cfg.Agent.Build.Provider = "existing-package"
	if len((&Kubernetes{}).Validate(cfg)) == 0 {
		t.Fatal("incompatible provider accepted")
	}
}

func TestBinaryPreparationFailurePreservesWorkingFilesAndConfig(t *testing.T) {
	entry, cfg := routingEntry(t)
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, "tasks"), 0700)
	os.WriteFile(filepath.Join(repo, "tasks", "agent.py"), nil, 0600)
	section, _ := json.Marshal(map[string]string{"repository": repo})
	cfg.Agent.Build = &config.BuildSelection{Provider: "invoke-binary", Section: section}
	originalConfig, _ := os.ReadFile(entry.ConfigPath())
	os.WriteFile(filepath.Join(entry.Dir, "agent.yaml"), []byte("keep working configuration"), 0600)
	builds := 0
	b := &Binary{adapter: agentbuild.Adapter{Run: func(_ context.Context, i agentbuild.Invocation) ([]byte, error) {
		if i.Program == "docker" {
			if i.Args[0] == "rm" {
				return nil, nil
			}
			if i.Args[0] == "image" {
				return json.Marshal([]map[string]string{{"Id": "sha256:" + strings.Repeat("a", 64), "Os": localTarget().OS, "Architecture": localTarget().Arch}})
			}
			return []byte(`{"abi":"python3.13","path":"/opt/datadog-agent/embedded/lib/python3.13/site-packages","os":"ubuntu","version":"24.04"}`), nil
		}
		builds++
		if i.Program != "dda" {
			t.Fatalf("unexpected preparation: %+v", i)
		}
		return nil, errors.New("compilation failed")
	}}, docker: func(...string) (string, error) { t.Fatal("stopped old Agent on preparation failure"); return "", nil }}
	if err := b.Install(cfg, entry); err == nil {
		t.Fatal("failed compilation accepted")
	}
	if builds != 1 {
		t.Fatal("build not attempted")
	}
	after, _ := os.ReadFile(entry.ConfigPath())
	if string(after) != string(originalConfig) {
		t.Fatal("stored source changed on failure")
	}
	live, _ := os.ReadFile(filepath.Join(entry.Dir, "agent.yaml"))
	if string(live) != "keep working configuration" {
		t.Fatal("live configuration changed on build failure")
	}
}
func TestSkipBuildNeverFallsBackToSource(t *testing.T) {
	entry, cfg := routingEntry(t)
	b := &Binary{docker: func(...string) (string, error) {
		t.Fatal("activation attempted without installed receipt")
		return "", nil
	}, adapter: agentbuild.Adapter{Run: func(context.Context, agentbuild.Invocation) ([]byte, error) {
		t.Fatal("skip-build prepared source")
		return nil, nil
	}}}
	if err := b.Update(cfg, entry, true); err == nil || !strings.Contains(err.Error(), "verified installed artifact missing") {
		t.Fatal(err)
	}
}
func TestPreparedBinaryMountsDurableGenerationNotWorktree(t *testing.T) {
	entry, _ := routingEntry(t)
	bundle := agentbuild.BinaryBundle{Executable: agentbuild.File{Path: "/pins/generation-1/agent"}, Runtime: agentbuild.Tree{Root: "/pins/generation-1/runtime"}, RuntimePrefix: "/build/repo/dev/embedded", Assets: agentbuild.Tree{Root: "/pins/generation-1/assets"}, RuntimeImageID: "sha256:" + strings.Repeat("a", 64), PythonPath: "/opt/datadog-agent/embedded/lib/python3.12/site-packages"}
	args := strings.Join(preparedBinaryRunArgs(entry, bundle), " ")
	for _, want := range []string{"/pins/generation-1/runtime:/build/repo/dev/embedded:ro", "--pull=never", "PYTHONPATH=" + bundle.PythonPath, "LD_LIBRARY_PATH=/build/repo/dev/embedded/lib"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %s: %s", want, args)
		}
	}
	if strings.Contains(args, "dev-lib") || strings.Contains(args, "/build/repo/dev/embedded:/") {
		t.Fatal("legacy/source runtime mounted")
	}
}
