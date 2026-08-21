// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestParseOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(podUIDEnv, "pod-uid")
	waitFile := filepath.Join(dir, "token")

	opts, err := parseOptions([]string{"--component", "trace-agent", "--state-dir", dir, "--wait-file", waitFile, "--", "trace-agent", "--config", "/etc/datadog-agent/datadog.yaml"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.component != "trace-agent" || opts.podUID != "pod-uid" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if len(opts.command) != 3 || opts.command[0] != "trace-agent" {
		t.Fatalf("unexpected command: %#v", opts.command)
	}
	if opts.lockPath != filepath.Join(dir, "trace-agent.lock") || opts.preparedPath != filepath.Join(dir, "trace-agent.prepared") || opts.activePath != filepath.Join(dir, "trace-agent.active") {
		t.Fatalf("unexpected derived paths: %#v", opts)
	}
	if opts.waitFile != waitFile {
		t.Fatalf("unexpected wait file: %q", opts.waitFile)
	}
}

func TestParseOptionsRejectsInvalidComponent(t *testing.T) {
	t.Setenv(podUIDEnv, "pod-uid")

	if _, err := parseOptions([]string{"--component", "../trace-agent", "--state-dir", t.TempDir(), "--", "trace-agent"}, io.Discard); err == nil {
		t.Fatal("expected a component path to be rejected")
	}
}

func TestParseOptionsHonorsCompleteLegacyStatePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(legacyLockPathEnv, filepath.Join(dir, "agent.lock"))
	t.Setenv(legacyPreparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(legacyActivePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")

	opts, err := parseOptions([]string{"--component", "agent", "--", "agent", "run"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.lockPath != filepath.Join(dir, "agent.lock") || opts.preparedPath != filepath.Join(dir, "agent.prepared") || opts.activePath != filepath.Join(dir, "agent.active") {
		t.Fatalf("legacy paths were not preserved: %#v", opts)
	}
}

func TestResolveStatePathsDefaultsAndRejectsPartialLegacyConfiguration(t *testing.T) {
	t.Setenv(legacyLockPathEnv, "")
	t.Setenv(legacyPreparedPathEnv, "")
	t.Setenv(legacyActivePathEnv, "")
	lockPath, preparedPath, activePath, err := resolveStatePaths("agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if lockPath != filepath.Join(defaultStateDir, "agent.lock") || preparedPath != filepath.Join(defaultStateDir, "agent.prepared") || activePath != filepath.Join(defaultStateDir, "agent.active") {
		t.Fatalf("unexpected default paths: %q, %q, %q", lockPath, preparedPath, activePath)
	}

	t.Setenv(legacyLockPathEnv, filepath.Join(t.TempDir(), "agent.lock"))
	if _, _, _, err := resolveStatePaths("agent", ""); err == nil {
		t.Fatal("expected partial legacy state paths to be rejected")
	}
}

func TestParseProbeOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "192.0.2.10")

	opts, err := parseProbeOptions([]string{"--component", "agent", "--state-dir", dir, "--kind", "startup", "--handler", "http", "--port", "5555", "--path", "/ready", "--timeout", "2s", "--failure-threshold", "6", "--termination-grace-period", "2m"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.kind != "startup" || opts.handler != "http" || opts.port != 5555 || opts.path != "/ready" || opts.address != "192.0.2.10" || opts.failureThreshold != 6 || opts.terminationGracePeriod != 2*time.Minute {
		t.Fatalf("unexpected probe options: %#v", opts)
	}
}

func TestParseProbeOptionsRejectsInvalidRestartPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(podUIDEnv, "pod-uid")

	base := []string{"--component", "agent", "--state-dir", dir, "--kind", "startup"}
	if _, err := parseProbeOptions(append(base, "--failure-threshold", "0"), io.Discard); err == nil {
		t.Fatal("expected a zero failure threshold to be rejected")
	}
	if _, err := parseProbeOptions(append(base, "--termination-grace-period", "0s"), io.Discard); err == nil {
		t.Fatal("expected a zero termination grace period to be rejected")
	}
}

func TestParseProbeOptionsRejectsUnsupportedHandler(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "192.0.2.10")

	if _, err := parseProbeOptions([]string{"--component", "agent", "--state-dir", dir, "--kind", "startup", "--handler", "exec"}, io.Discard); err == nil {
		t.Fatal("expected unsupported handler to be rejected")
	}
}

func TestParseProbeOptionsRejectsInvalidPodIP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "not-an-ip")

	if _, err := parseProbeOptions([]string{"--component", "agent", "--state-dir", dir, "--kind", "startup", "--handler", "tcp", "--port", "5555"}, io.Discard); err == nil {
		t.Fatal("expected an invalid Pod IP to be rejected")
	}
}

func TestAgentEnvironmentRemovesOnlyRolloutMetadata(t *testing.T) {
	environment := agentEnvironment([]string{
		legacyLockPathEnv + "=/state/agent.lock",
		podUIDEnv + "=pod-uid",
		"DD_CMD_PORT=5002",
		legacyPreparedPathEnv + "=/state/agent.prepared",
		podIPEnv + "=192.0.2.10",
		legacyActivePathEnv + "=/state/agent.active",
		"PATH=/bin",
	})
	want := []string{"DD_CMD_PORT=5002", "PATH=/bin"}
	if len(environment) != len(want) || environment[0] != want[0] || environment[1] != want[1] {
		t.Fatalf("cleaned environment = %#v, want %#v", environment, want)
	}
}
