// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestListShowsFirstAndUpdateActivationFailuresWithoutInstalledFlag(t *testing.T) {
	store := lifecycleEnv(t) // no Docker, credentials, or executor on PATH
	cfg, errs := config.Parse([]byte("schema: 1\nenvironment: {base: local}\nagent: {install: binary}\n"))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, tc := range []struct{ name, phase, id string }{{"first-failed", "failed", ""}, {"update-failed", "failed", "previous-receipt"}, {"in-progress", "activating", ""}} {
		entry, err := store.Create(tc.name, cfg, envstore.Meta{Status: envstore.StatusReady, AgentInstalled: false, AgentArtifactID: tc.id})
		if err != nil {
			t.Fatal(err)
		}
		if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{}, map[string]any{"_agent_artifact": map[string]any{"phase": tc.phase}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Create("never-installed", cfg, envstore.Meta{Status: envstore.StatusReady}); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	previous := os.Stdout
	os.Stdout = output
	defer func() { os.Stdout = previous }()
	if err := cmdList(nil); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	raw, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	for name, want := range map[string]string{"first-failed": "artifact failed", "update-failed": "artifact failed", "in-progress": "artifact activating", "never-installed": "-"} {
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, name+" ") {
				found = true
				if !strings.Contains(line, want) {
					t.Fatalf("%s missing %s: %s", name, want, line)
				}
			}
		}
		if !found {
			t.Fatal("missing environment", name)
		}
	}
}
