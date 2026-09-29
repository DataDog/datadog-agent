// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresBundlesAndNeverCaptures(t *testing.T) {
	dir := t.TempDir()
	scenario := filepath.Join(dir, "scenario.yaml")
	config := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(scenario, []byte("version: 1\nscenario: {name: healthy}\nexpectation: {conclusion: healthy}\nfleet: [{group: mac, os: macos, count: 5}]\nphases: [{name: healthy, duration: 20m}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("site: datad0g.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DD_SITE", "datad0g.com")
	called := false
	cmd := MakeCommand(Runtime{Capture: func(context.Context, CaptureRequest) error { called = true; return nil }, Replay: func(context.Context, ReplayRequest) error { called = true; return nil }})
	cmd.SetArgs([]string{"run", "--scenario", scenario, "--config", config})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--bundle mac=directory") {
		t.Fatalf("expected actionable missing-bundle error, got %v", err)
	}
	if called {
		t.Fatal("missing evidence started capture or delivery")
	}
}

func TestCaptureDeadlineAndSubcommands(t *testing.T) {
	cmd := MakeCommand(Runtime{})
	for _, name := range []string{"capture", "validate", "plan", "run"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil || sub == cmd {
			t.Fatalf("missing command %s", name)
		}
		if sub.Flags().Lookup("fast") != nil {
			t.Fatal("staging time acceleration is exposed")
		}
	}
	capture, _, _ := cmd.Find([]string{"capture"})
	if capture.Flags().Lookup("deadline").DefValue != "35m0s" {
		t.Fatal("capture does not cover host-metadata cadence")
	}
}
