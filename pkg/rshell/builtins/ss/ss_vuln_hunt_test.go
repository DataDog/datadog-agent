// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ss_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntSubsystemInvariantViolation_SSPathLikeOperandsNotOpened(t *testing.T) {
	pathOperand := filepath.Join(t.TempDir(), "attacker-controlled-proc-root")
	stdout, stderr, code := cmdRun(t, "ss -- "+strconv.Quote(pathOperand))

	assert.True(t, code == 0 || code == 1, "unexpected exit code %d", code)
	assert.NotContains(t, stdout, pathOperand)
	assert.NotContains(t, stderr, pathOperand)
	assert.NotContains(t, stderr, "attacker-controlled-proc-root")
}

func TestVulnHuntBuiltinFlagDrivenExploit_DangerousFlagsWithArgumentsRejected(t *testing.T) {
	cases := []string{
		"ss --filter=/etc/passwd",
		"ss -F/etc/passwd",
		"ss --net=/proc/1/ns/net",
		"ss -N/proc/1/ns/net",
		"ss --processes",
		"ss --kill",
		"ss --events",
		"ss --resolve",
		"ss --bpf",
	}

	for _, script := range cases {
		t.Run(script, func(t *testing.T) {
			stdout, stderr, code := cmdRun(t, script)
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "ss:")
		})
	}
}
