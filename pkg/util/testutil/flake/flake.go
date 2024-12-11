// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flake marks an instance of [testing.TB](https://pkg.go.dev/testing#TB) as flake.
// Use [flake.Mark] to mark a known flake test.
// Use `skip-flake` to control the behavior, or set the environment variable `SKIP_FLAKE`.
// Flags take precedence over environment variables.
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
// Otherwise test will be marked as known flake through a special message on tests output.
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
		return false
	}
	return shouldSkipFlakeVar
}
