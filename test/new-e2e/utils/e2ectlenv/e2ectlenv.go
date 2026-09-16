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

// Attach returns a provisioner binding the test environment to the named
// e2ectl environment's snapshot — one line per entry point:
//
//	envName := e2ectlenv.RequireEnv(t)
//	e2e.Run(t, &mySuite{}, e2e.WithProvisioner(e2ectlenv.Attach[environments.Host](envName)))
func Attach[Env any](envName string) *provisioner.StaticStackProvisioner[Env] {
	return provisioner.NewStaticStackProvisioner[Env]("e2ectl-attach", SnapshotPath(envName))
}
