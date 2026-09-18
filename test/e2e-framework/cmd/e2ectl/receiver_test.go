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
	path := writeLifecycleConfig(t, "schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  install: binary\n  receiver:\n    type: blackhole\n    blackhole: {url: 'http://sink:8080'}\n")
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
