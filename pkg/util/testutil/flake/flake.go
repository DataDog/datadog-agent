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
	"strconv"
	"testing"
)

const flakyTestMessage = "flakytest: this is a known flaky test"

var skipFlake = flag.Bool("skip-flake", false, "skip tests labeled as flakes")

// Mark test as a known flaky.
// If any of skip-flake flag or GO_TEST_SKIP_FLAKE environment variable is set, the test will be skipped.
// Otherwise test will be retried on failure.
func Mark(t testing.TB) {
	t.Helper()
	if shouldSkipFlake() {
		t.Skip("flakytest: skip known flaky test")
		return
	}
	t.Log(flakyTestMessage)
}

func shouldSkipFlake() bool {
	if *skipFlake {
		return true
	}
	shouldSkipFlakeVar, err := strconv.ParseBool(os.Getenv("GO_TEST_SKIP_FLAKE"))
	if err != nil {
		return shouldSkipFlakeVar
	}
	return false
}
