// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package df_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

// TestDfNotSupportedOnWindows asserts that df returns a clear "not
// supported" error and a non-zero exit code on Windows. v1 only supports
// Linux and macOS.
func TestDfNotSupportedOnWindows(t *testing.T) {
	stdout, stderr, code := testutil.RunScript(t, "df", "")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "df:")
	assert.Contains(t, stderr, "not supported")
}

// TestDfHelpAlwaysWorks asserts that --help works on every platform so
// that scripts can introspect df without first failing on enumeration.
func TestDfHelpAlwaysWorks(t *testing.T) {
	stdout, _, code := testutil.RunScript(t, "df --help", "")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: df")
}
