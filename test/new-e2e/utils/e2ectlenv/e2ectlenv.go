// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2ectlenv gives new-e2e tests a one-line attach to a live,
// e2ectl-owned environment (e2ectl start + e2ectl install). Tests that
// attach call RequireEnv in their entry point and pass the snapshot to
// Run; the test body stays unchanged between CI and local runs.
//
// Convention: attachable entry points are named <Test>On<Local|LocalKind|Host>
// so `e2ectl test` can pre-select the right one per environment base with
// its default -run pattern, keeping provisioning-based entries from firing.
package e2ectlenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// EnvVar is the environment variable carrying the e2ectl environment name.
// `e2ectl test` sets it; manual `go test` invocations can too.
const EnvVar = "E2ECTL_ENV"

// HomeVar is the environment variable locating the e2ectl store.
const HomeVar = "E2ECTL_HOME"

// Attached reports whether the test process runs attached to a live e2ectl
// environment — `e2ectl test` sets E2ECTL_ENV. Tests use it to draw the
// attach-mode boundary explicitly (e.g. skipping UpdateEnv re-provisioning)
// instead of failing on the provisioner mismatch.
func Attached() bool { return os.Getenv(EnvVar) != "" }

// Home returns the e2ectl store root: $E2ECTL_HOME, default ~/.e2ectl.
func Home() string {
	if home := os.Getenv(HomeVar); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".e2ectl")
}

// SnapshotPath returns the snapshot path of a named environment.
func SnapshotPath(envName string) string {
	return filepath.Join(Home(), "envs", envName, "snapshot.json")
}

// RequireEnv returns the e2ectl environment name from E2ECTL_ENV, or skips
// the test when unset. Attachable entry points call this first.
func RequireEnv(t *testing.T) string {
	t.Helper()
	envName := os.Getenv(EnvVar)
	if envName == "" {
		t.Skipf("set %s to a running e2ectl environment (e2ectl test -env <name> sets it automatically)", EnvVar)
	}
	return envName
}

// RequireSnapshot returns the snapshot path of a named environment, or
// skips the test when no snapshot exists (e2ectl start first).
func RequireSnapshot(t *testing.T, envName string) string {
	t.Helper()
	snapshot := SnapshotPath(envName)
	if _, err := os.Stat(snapshot); err != nil {
		t.Skipf("no snapshot for %s (e2ectl start first): %v", envName, err)
	}
	return snapshot
}

// RequireBase skips the test when the named environment's base differs from
// the entry point's affix (OnLocalKind, OnEKS, OnHost, ...). Entry points
// carry their base in the name so a broad -run pattern still attaches each
// suite to the base it was written for — the wrong-base entry skips instead
// of running against an environment it may not fit.
func RequireBase(t *testing.T, envName, want string) {
	t.Helper()
	metaPath := filepath.Join(Home(), "envs", envName, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Skipf("no environment metadata for %s (e2ectl start first): %v", envName, err)
	}
	var meta struct {
		Base string `json:"base"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Skipf("unreadable environment metadata for %s: %v", envName, err)
	}
	if meta.Base != want {
		t.Skipf("environment %s is base %q; this entry point targets base %q", envName, meta.Base, want)
	}
}

// Attach returns a provisioner binding the test environment to the named
// e2ectl environment's snapshot — one line per entry point:
//
//	envName := e2ectlenv.RequireEnv(t)
//	e2e.Run(t, &mySuite{}, e2e.WithProvisioner(e2ectlenv.Attach[environments.Host](envName)))
func Attach[Env any](envName string) *provisioner.StaticStackProvisioner[Env] {
	return provisioner.NewStaticStackProvisioner[Env]("e2ectl-attach", SnapshotPath(envName))
}
