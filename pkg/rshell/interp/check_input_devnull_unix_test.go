// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package interp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The native null device is exempt from the input-redirect cat rule in Check,
// as in Run.
func TestCheckInputRedirectNativeDevNullNotGatedOnCat(t *testing.T) {
	runner := checkRunner(t, AllowedCommands([]string{"rshell:tr"}), AllowedPaths([]string{"/dev"}))
	result := checkScript(t, runner, "tr a-z A-Z < /dev/null")
	assert.False(t, checkHasIssue(result, CheckCommandNotAllowed), "%+v", result)
	assert.Equal(t, CheckAllowed, result.Status, "%+v", result)
}
