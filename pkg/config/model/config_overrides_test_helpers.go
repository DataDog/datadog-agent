// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package model

import "testing"

// CleanOverride registers a function to clean the overrides vars and func from the configuration at the end of a test.
func CleanOverride(t *testing.T) {
	t.Cleanup(func() {
		overrideVars = make(map[string]interface{})
		overrideFuncs = make([]func(Config), 0)
	})
}
