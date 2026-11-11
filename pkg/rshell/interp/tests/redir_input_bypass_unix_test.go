// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package tests_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPentestInputRedirect_NativeDevNullNotGatedOnCat verifies that the
// native null device stays usable as input without cat: with /dev in
// AllowedPaths the redirect succeeds and yields empty input.
func TestPentestInputRedirect_NativeDevNullNotGatedOnCat(t *testing.T) {
	stdout, stderr, code := runBypass(t,
		`tr A-Z a-z < /dev/null; echo "rc=$?"`,
		"/dev",
		[]string{"rshell:tr", "rshell:echo"},
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "rc=0\n", stdout, "null input must be empty and succeed")
	assert.Empty(t, stderr)
}
