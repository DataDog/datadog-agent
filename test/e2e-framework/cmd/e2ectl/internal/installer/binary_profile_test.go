// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

func TestUnprofiledBinaryRejectedBeforePreparationOrActivation(t *testing.T) {
	for _, operation := range []string{"install", "reuse", "apply"} {
		t.Run(operation, func(t *testing.T) {
			entry, cfg := routingEntry(t)
			r := namedRuntimeArtifact(t)
			r.Profile = nil
			if err := r.Validate(r.Target); err != nil {
				t.Fatalf("unprofiled legacy bundle should remain structurally usable: %v", err)
			}
			manifest := filepath.Join(t.TempDir(), "result.json")
			if err := agentbuild.Write(manifest, r); err != nil {
				t.Fatal(err)
			}
			// The build selection is derived/internal now: install/reuse pin it
			// directly to exercise the same installer gate (unprofiled receipts
			// rejected); apply must keep the derived selection so the routing-only
			// change check sees the stored config's identical derivation.
			cfg.Agent.Install = "binary"
			cfg.Agent.Section = []byte("{}\n")
			if operation != "apply" {
				cfg.Agent.Build = &config.BuildSelection{Provider: "existing-binary", Section: []byte("manifest: " + manifest + "\n")}
			}
			if err := os.WriteFile(entry.ConfigPath(), cfg.Source(), 0600); err != nil {
				t.Fatal(err)
			}
			if err := publishArtifact(entry, r, nil); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(entry.SnapshotPath())
			if err != nil {
				t.Fatal(err)
			}
			b := &Binary{adapter: agentbuild.Adapter{Run: func(context.Context, agentbuild.Invocation) ([]byte, error) {
				t.Fatal("unprofiled bundle triggered probe/build")
				return nil, nil
			}}, docker: func(...string) (string, error) { t.Fatal("unprofiled bundle reached activation"); return "", nil }}
			switch operation {
			case "install":
				err = b.Install(cfg, entry)
			case "reuse":
				err = b.Update(cfg, entry, true)
			case "apply":
				err = b.ApplyRouting(cfg, entry)
			}
			if err == nil || !strings.Contains(err.Error(), "capability receipt") {
				t.Fatal(err)
			}
			after, err := os.ReadFile(entry.SnapshotPath())
			if err != nil || string(after) != string(before) {
				t.Fatal("rejection changed installed snapshot", err)
			}
			if _, err := os.Stat(filepath.Join(entry.Dir, "agent.yaml")); !os.IsNotExist(err) {
				t.Fatal("rejection wrote live config")
			}
		})
	}
}
