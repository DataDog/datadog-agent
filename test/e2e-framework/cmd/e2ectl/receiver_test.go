// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/receiver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestLocalNoFixtureCreatesOnlyNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("local Docker driver support is Linux-only")
	}
	store := lifecycleEnv(t)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands")
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+log+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	path := writeLifecycleConfig(t, "schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  source: true\n  receiver:\n    type: blackhole\n    blackhole: {url: 'http://sink:8080'}\n")
	if err := cmdStart([]string{"--config", path, "--name", "empty"}); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Get("empty")
	if err != nil {
		t.Fatal(err)
	}
	resources, _, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil || len(resources) != 0 {
		t.Fatal(resources, err)
	}
	commands, err := os.ReadFile(log)
	if err != nil || strings.TrimSpace(string(commands)) != "network create empty-net" {
		t.Fatal(string(commands), err)
	}
	if err := cmdReceiver([]string{"plan", "--env", "empty"}); err != nil {
		t.Fatal(err)
	}
	// Offline plan must not create a sink or inspect Docker.
	again, _ := os.ReadFile(log)
	if string(again) != string(commands) {
		t.Fatal("planning contacted Docker")
	}
	if err := cmdStop([]string{"--env", "empty"}); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedManagedConfigsMatchSupportedProfiles(t *testing.T) {
	for _, base := range []string{"local", "kind", "ec2-host"} {
		raw, err := driver.StarterConfig(base)
		if err != nil {
			t.Fatal(err)
		}
		cfg, errs := config.Parse(raw)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if cfg.Agent.Receiver == nil || cfg.Agent.Receiver.Type != "fakeintake" {
			t.Fatal("starter did not select capture explicitly")
		}
		d, _ := driver.Get(base)
		inst, err := driver.InstallerFor(d, cfg.Agent.Install)
		if err != nil {
			t.Fatal(err)
		}
		if errs := inst.Validate(cfg); len(errs) > 0 {
			t.Fatal(errs)
		}
		if cfg.Agent.Install != "binary" && !strings.Contains(string(cfg.Agent.Section), "7.83.0") {
			t.Fatal("starter does not match release profile")
		}
	}
}

// The managed blackhole UX: `receiver: type: blackhole` with nothing else.
// Planning stays read-only and fails closed until install/apply syncs the
// environment-provided sink; switching the selection away removes it; stop
// always cleans the deterministic container up.
func TestLocalManagedBlackholeSinkLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("local Docker driver support is Linux-only")
	}
	store := lifecycleEnv(t)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands")
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+log+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	managed := "schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  source: true\n  receiver:\n    type: blackhole\n"
	path := writeLifecycleConfig(t, managed)
	if err := cmdStart([]string{"--config", path, "--name", "sink"}); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Get("sink")
	if err != nil {
		t.Fatal(err)
	}

	// Planning must not create the sink: it fails closed on the missing fact.
	planErr := cmdReceiver([]string{"plan", "--env", "sink"})
	if planErr == nil || !strings.Contains(planErr.Error(), "managed blackhole sink is not running") {
		t.Fatalf("plan without a sink must fail closed, got: %v", planErr)
	}
	before, _ := os.ReadFile(log)
	if !strings.HasSuffix(strings.TrimSpace(string(before)), "network create sink-net") {
		t.Fatalf("planning contacted Docker: %s", before)
	}

	// The apply path syncs the environment-provided sink before resolution. It
	// still fails on the missing installed agent, but only after starting the
	// sink — proving the agent cannot start against a sink that does not exist.
	applyErr := cmdReceiver([]string{"apply", "--env", "sink", "--config", path})
	if applyErr == nil {
		t.Fatal("apply without an installed agent must still fail")
	}
	commands, _ := os.ReadFile(log)
	joined := strings.TrimSpace(string(commands))
	if !strings.Contains(joined, "rm -f sink-blackhole") {
		t.Fatalf("sync must replace any previous sink first: %s", joined)
	}
	runCmd := "run -d --name sink-blackhole --network sink-net --read-only --cap-drop=ALL --security-opt no-new-privileges --no-healthcheck -v " +
		filepath.Join(entry.Dir, "sink", "e2ectl") + ":/tool:ro --entrypoint /tool " +
		"registry.datadoghq.com/agent:7.83.0 receiver serve --type blackhole --listen 0.0.0.0:8080"
	if !strings.Contains(joined, runCmd) {
		t.Fatalf("sync must start the managed sink container exactly once with pinned boundaries: %s", joined)
	}
	var sinkFact receiver.BlackholeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "blackhole", &sinkFact); err != nil {
		t.Fatal(err)
	}
	if sinkFact.AgentURL != "http://sink-blackhole:8080" {
		t.Fatalf("snapshot fact must carry the in-network URL: %+v", sinkFact)
	}

	// With the fact present, the same plan resolves without touching Docker.
	if err := cmdReceiver([]string{"plan", "--env", "sink"}); err != nil {
		t.Fatal(err)
	}
	if planAgain, _ := os.ReadFile(log); string(planAgain) != string(commands) {
		t.Fatal("resolving the managed sink must not restart it")
	}

	// Switching the selection away removes the sink and tombstones the fact.
	away := writeLifecycleConfig(t, "schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  source: true\n  receiver:\n    type: fakeintake\n")
	d, err := driver.Get("local")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(away)
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.SyncManagedSink(d.SinkSyncer(), cfg, entry); err != nil {
		t.Fatal(err)
	}
	var gone receiver.BlackholeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "blackhole", &gone); err != nil {
		t.Fatal(err)
	}
	if gone.AgentURL != "" {
		t.Fatalf("switching away must tombstone the sink fact: %+v", gone)
	}
	if planErr := cmdReceiver([]string{"plan", "--env", "sink"}); planErr == nil {
		t.Fatal("tombstoned sink fact must not resolve")
	}

	// Teardown removes the deterministic sink container with everything else.
	if err := cmdStop([]string{"--env", "sink"}); err != nil {
		t.Fatal(err)
	}
	final, _ := os.ReadFile(log)
	if !strings.Contains(string(final), "rm -f sink-blackhole") {
		t.Fatal("stop must clean up the managed sink container")
	}
}
