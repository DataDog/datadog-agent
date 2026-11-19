// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package model

import (
	"slices"
	"testing"
)

// CleanOverride registers a function to clean the overrides vars and func from the configuration at the end of a test.
func CleanOverride(t *testing.T) {
	t.Cleanup(func() {
		overrideFuncs = make([]func(Config), 0)
	})
}

// RestoreOverrideFuncs snapshots the override functions and returns a function setting them back to the snapshot,
// which also runs at the end of the test. Tests that initialize the configuration again call it once that's done:
// initializing registers most override functions again, but not the ones registered once per process (sync.Once),
// so the snapshot taken before is the one holding each override function exactly once.
func RestoreOverrideFuncs(t testing.TB) (restore func()) {
	saved := slices.Clone(overrideFuncs)
	restore = func() {
		overrideFuncs = slices.Clone(saved)
	}
	t.Cleanup(restore)
	return restore
}
