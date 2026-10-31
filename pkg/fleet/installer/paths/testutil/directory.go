// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides directory fixtures for installer configuration trust tests.
// Fixtures do not change installer path globals. Ownership is enforced on Windows;
// other platforms use ordinary temporary directories.
package testutil

import "testing"

// TrustedDir returns a temporary directory with a trusted Windows owner.
func TrustedDir(t testing.TB) string {
	t.Helper()
	return temporaryDir(t, true)
}

// UntrustedDir returns a temporary directory with an untrusted Windows owner.
func UntrustedDir(t testing.TB) string {
	t.Helper()
	return temporaryDir(t, false)
}
