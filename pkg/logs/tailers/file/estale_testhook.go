// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package file

import "errors"

// ErrStaleFileHandleTest is a sentinel error for cross-platform stale-handle tests.
var ErrStaleFileHandleTest = errors.New("stale file handle test sentinel")

// SetStaleFileHandleHookForTest overrides IsStaleFileHandle for the duration of a test.
func SetStaleFileHandleHookForTest(hook func(error) bool) func() {
	prev := staleFileHandleHook
	staleFileHandleHook = hook
	return func() {
		staleFileHandleHook = prev
	}
}
