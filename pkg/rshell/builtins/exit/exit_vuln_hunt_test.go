// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package exit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// Campaign: vuln-hunt/2026-05-19-codex. These are public-safe blocked-attack
// regressions for exit.

func TestVulnHuntBuiltinFileAccessBypass_PathOperandsAreNumericData(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := testutil.RunScript(t,
		"exit /etc/passwd", dir,
		interp.AllowedPaths([]string{dir}))

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "/etc/passwd: numeric argument required")
}

func TestVulnHuntBuiltinResourceExhaustion_HugeInvalidNumericOperand(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	arg := strings.Repeat("9", 64*1024)
	stdout, stderr, code := testutil.RunScriptCtx(ctx, t,
		"exit "+arg, dir,
		interp.AllowedPaths([]string{dir}))

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "numeric argument required")
	assert.LessOrEqual(t, len(stderr), len(arg)+64)
}
