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
	t.Setenv(lockPathEnv, filepath.Join(dir, "trace-agent.lock"))
	t.Setenv(preparedPathEnv, filepath.Join(dir, "trace-agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "trace-agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")

	opts, err := parseOptions([]string{"--component", "trace-agent", "--wait-file", "/etc/datadog-agent/auth/token", "--", "trace-agent", "--config", "/etc/datadog-agent/datadog.yaml"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.component != "trace-agent" || opts.podUID != "pod-uid" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if len(opts.command) != 3 || opts.command[0] != "trace-agent" {
		t.Fatalf("unexpected command: %#v", opts.command)
	}
	if opts.waitFile != "/etc/datadog-agent/auth/token" {
		t.Fatalf("unexpected wait file: %q", opts.waitFile)
	}
}

func TestParseOptionsRejectsAnotherComponentPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(lockPathEnv, filepath.Join(dir, "agent.lock"))
	t.Setenv(preparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")

	if _, err := parseOptions([]string{"--component", "trace-agent", "--", "trace-agent"}, io.Discard); err == nil {
		t.Fatal("expected mismatched component paths to be rejected")
	}
}

func TestParseProbeOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(preparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "192.0.2.10")

	opts, err := parseProbeOptions([]string{"--kind", "startup", "--handler", "http", "--port", "5555", "--path", "/ready", "--timeout", "2s", "--failure-threshold", "6", "--termination-grace-period", "2m"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.kind != "startup" || opts.handler != "http" || opts.port != 5555 || opts.path != "/ready" || opts.address != "192.0.2.10" || opts.failureThreshold != 6 || opts.terminationGracePeriod != 2*time.Minute {
		t.Fatalf("unexpected probe options: %#v", opts)
	}
}

func TestParseProbeOptionsRejectsInvalidRestartPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(preparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")

	if _, err := parseProbeOptions([]string{"--kind", "startup", "--failure-threshold", "0"}, io.Discard); err == nil {
		t.Fatal("expected a zero failure threshold to be rejected")
	}
	if _, err := parseProbeOptions([]string{"--kind", "startup", "--termination-grace-period", "0s"}, io.Discard); err == nil {
		t.Fatal("expected a zero termination grace period to be rejected")
	}
}

func TestParseProbeOptionsRejectsUnsupportedHandler(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(preparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "192.0.2.10")

	if _, err := parseProbeOptions([]string{"--kind", "startup", "--handler", "exec"}, io.Discard); err == nil {
		t.Fatal("expected unsupported handler to be rejected")
	}
}

func TestParseProbeOptionsRejectsInvalidPodIP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(preparedPathEnv, filepath.Join(dir, "agent.prepared"))
	t.Setenv(activePathEnv, filepath.Join(dir, "agent.active"))
	t.Setenv(podUIDEnv, "pod-uid")
	t.Setenv(podIPEnv, "not-an-ip")

	if _, err := parseProbeOptions([]string{"--kind", "startup", "--handler", "tcp", "--port", "5555"}, io.Discard); err == nil {
		t.Fatal("expected an invalid Pod IP to be rejected")
	}
}
