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
)

// lifecycleEnv isolates a hermetic lifecycle test: a private store, a missing
// executor and an empty PATH. Starts fail before any resource exists and
// teardowns are exercised without kind, Docker or Pulumi.
func lifecycleEnv(t *testing.T) *envstore.Store {
	t.Helper()
	t.Setenv("E2ECTL_HOME", t.TempDir())
	t.Setenv("E2ECTL_WORKER", "/nonexistent/executor")
	t.Setenv("PATH", "")
	store, err := envstore.New()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeLifecycleConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "env.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A failed start (kind binary absent) must mark the entry error — never leave
// it stuck in "provisioning" — and plain `stop` must then recover it.
func TestFailedStartMarksErrorAndStopRecovers(t *testing.T) {
	store := lifecycleEnv(t)
	cfgPath := writeLifecycleConfig(t, `schema: 1
environment:
  base: kind
agent:
  install: helm
`)
	if err := cmdStart([]string{"--config", cfgPath, "--name", "dev"}); err == nil {
		t.Fatal("start must fail without kind on PATH")
	}
	entry, err := store.Get("dev")
	if err != nil {
		t.Fatal("the failed entry must stay visible in the store")
	}
	if entry.Meta.Status != envstore.StatusError {
		t.Fatalf("failed start must mark the entry error, got %q", entry.Meta.Status)
	}
	if err := cmdStop([]string{"--env", "dev"}); err != nil {
		t.Fatalf("stop must recover a failed entry: %v", err)
	}
	if _, err := store.Get("dev"); err == nil {
		t.Fatal("stop must remove the failed entry")
	}
}

// A failed EC2 start (executor missing): stop without --force keeps the entry
// and points at --force; stop --force removes it with a leftover warning.
func TestStopForceRemovesEntryWhenTeardownFails(t *testing.T) {
	store := lifecycleEnv(t)
	cfg, errs := config.Parse([]byte(`schema: 1
environment:
  base: ec2-host
  ec2-host:
    os: ubuntu-22.04
agent:
  install: script
`))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, err := store.Create("vm", cfg, envstore.Meta{Status: envstore.StatusError}); err != nil {
		t.Fatal(err)
	}

	err := cmdStop([]string{"--env", "vm"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("a failed teardown must keep the entry and point at --force, got: %v", err)
	}
	if _, getErr := store.Get("vm"); getErr != nil {
		t.Fatal("without --force the entry must remain for recovery")
	}
	if err := cmdStop([]string{"--env", "vm", "--force"}); err != nil {
		t.Fatalf("--force must remove the entry: %v", err)
	}
	if _, getErr := store.Get("vm"); getErr == nil {
		t.Fatal("--force must remove the entry from the store")
	}
}

func TestStopUnknownEnvironment(t *testing.T) {
	lifecycleEnv(t)
	err := cmdStop([]string{"--env", "nope"})
	if err == nil || !strings.Contains(err.Error(), "no environment named") {
		t.Fatalf("expected an unknown-environment error, got: %v", err)
	}
}
