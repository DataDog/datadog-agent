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

// RestoreOverrideFuncs empties the override functions and registers a function restoring them at the end of the test.
// Tests that initialize the configuration again, registering its override functions again, use it so each override
// function is registered, and runs, once.
func RestoreOverrideFuncs(t testing.TB) {
	saved := slices.Clone(overrideFuncs)
	overrideFuncs = make([]func(Config), 0)
	t.Cleanup(func() {
		overrideFuncs = saved
	})
}
