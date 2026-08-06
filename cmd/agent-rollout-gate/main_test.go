// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"io"
	"path/filepath"
	"testing"
)

func TestParseOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(lockPathEnv, filepath.Join(dir, "trace-agent.lock"))
	t.Setenv(preparedPathEnv, filepath.Join(dir, "trace-agent.prepared"))
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
	t.Setenv(podUIDEnv, "pod-uid")

	if _, err := parseOptions([]string{"--component", "trace-agent", "--", "trace-agent"}, io.Discard); err == nil {
		t.Fatal("expected mismatched component paths to be rejected")
	}
}
