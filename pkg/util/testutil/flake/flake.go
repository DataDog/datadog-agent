// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flake provides a [testing.TB](https://pkg.go.dev/testing#TB) implementation that support flaky tests.
// Flaky tests are tests that are not reliable and may fail for various reasons.
// Wrap your *testing.T with [flake.FlakyTesting] to allow flaky tests to fail or skip them.
// Use flags `allow-flake-failure` and `skip-flake` to control the behavior, or set the environment variables `ALLOW_FLAKE_FAILURE` and `SKIP_FLAKE`.
// If both `allow-flake-failure` and `skip-flake` are set, `skip-flake` takes precedence.
// Flags take precedence over environment variables.
// Subtests from [testing.T.Run](https://pkg.go.dev/testing#T.Run) should be wrapped with [flake.Wrap](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/util/testutil/flake#Wrap) to inherit the behavior.
package flake

import (
	"flag"
	"os"
	"testing"
)

var allowFlakeFailure = flag.Bool("allow-flake-failure", false, "allow flake tests failures")
var skipFlake = flag.Bool("skip-flake", false, "skip tests labeled as flakes")

type allowflakeTesting struct {
	testing.TB
}

// Wrap wraps a testing.TB to allow flaky tests to fail or skip them.
func Wrap(t testing.TB) testing.TB {
	// skip flake takes precedence over allow flake failure
	if shouldAllowFlake() && shouldSkipFlake() {
		t.Log("skip flake and allow flake failure are both set, skip flake takes precedence")
	}
	if shouldSkipFlake() {
		t.Skip("skip test marked as flake")
		return t
	}
	if shouldAllowFlake() {
		t.Log("🟡 allow flake failure")
		return &allowflakeTesting{t}
	}
	return t
}

func shouldAllowFlake() bool {
	if *allowFlakeFailure {
		return true
	}
	if os.Getenv("ALLOW_FLAKE_FAILURE") == "true" {
		return true
	}
	return false
}

func shouldSkipFlake() bool {
	if *skipFlake {
		return true
	}
	if os.Getenv("SKIP_FLAKE") == "true" {
		return true
	}
	return false
}

func (t *allowflakeTesting) Error(args ...any) {
	t.Log(args...)
	t.Skip("allowing error - test marked as flake")
}

func (t *allowflakeTesting) Errorf(format string, args ...any) {
	t.Logf(format, args...)
	t.Skip("allowing error - test marked as flake")
}

func (t *allowflakeTesting) Fail() {
	t.Skip("allowing failure - test marked as flake")
}

func (t *allowflakeTesting) FailNow() {
	t.Skip("allowing failure - test marked as flake")
}

func (t *allowflakeTesting) Fatal(args ...any) {
	t.Log(args...)
	t.Skip("allowing fatal - test marked as flake")
}

func (t *allowflakeTesting) Fatalf(format string, args ...any) {
	t.Logf(format, args...)
	t.Skip("allowing fatal - test marked as flake")
}
