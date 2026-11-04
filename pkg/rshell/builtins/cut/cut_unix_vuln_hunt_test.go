// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package cut_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H17: /dev/zero has no newline, so the scanner must stop at MaxLineBytes
// instead of reading forever when the operator explicitly allows the device.
func TestVulnHuntBuiltinSpecialFiles_DevZeroHitsLineCap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stdout, stderr, code := testutil.RunScriptCtx(ctx, t, "cut -b1 /dev/zero", "",
		interp.AllowedPaths([]string{"/dev/zero"}))

	require.NoError(t, ctx.Err())
	assert.NotEqual(t, 0, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "cut:")
}
