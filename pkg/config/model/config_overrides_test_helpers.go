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

// RestoreOverrideFuncs registers a function restoring, at the end of a test, the override functions registered when it
// is called. Tests that initialize the configuration again, registering its override functions again, use it so they
// don't pile up.
func RestoreOverrideFuncs(t testing.TB) {
	saved := slices.Clone(overrideFuncs)
	t.Cleanup(func() {
		overrideFuncs = saved
	})
}
