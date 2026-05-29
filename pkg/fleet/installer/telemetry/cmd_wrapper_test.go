// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracedCmdRunSuccess(t *testing.T) {
	ctx := context.Background()
	cmd := CommandContext(ctx, "sh", "-c", "exit 0")
	err := cmd.Run()
	require.NoError(t, err)
	assert.Equal(t, int32(0), cmd.span.span.Error)
}

func TestTracedCmdRunErrorUnexpected(t *testing.T) {
	ctx := context.Background()
	cmd := CommandContext(ctx, "sh", "-c", "exit 1")
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, int32(1), cmd.span.span.Error)
	assert.Equal(t, float64(1), cmd.span.span.Metrics["exit_code"])
	assert.NotContains(t, cmd.span.span.Meta, "expected_exit_code")
}

func TestTracedCmdRunErrorExpected(t *testing.T) {
	ctx := context.Background()
	cmd := CommandContext(ctx, "sh", "-c", "exit 1").WithExpectedExitCodes(1)
	err := cmd.Run()
	// The error is still returned to the caller
	require.Error(t, err)
	// But the span is not marked as an error
	assert.Equal(t, int32(0), cmd.span.span.Error)
	assert.Equal(t, float64(1), cmd.span.span.Metrics["exit_code"])
	assert.Equal(t, "true", cmd.span.span.Meta["expected_exit_code"])
}

func TestTracedCmdRunExpectedNonZeroAndZeroSuccess(t *testing.T) {
	ctx := context.Background()

	// expected non-zero exit: span not an error, but error still returned
	cmdErr := CommandContext(ctx, "sh", "-c", "exit 5").WithExpectedExitCodes(5)
	err := cmdErr.Run()
	require.Error(t, err)
	assert.Equal(t, int32(0), cmdErr.span.span.Error)
	assert.Equal(t, float64(5), cmdErr.span.span.Metrics["exit_code"])
	assert.Equal(t, "true", cmdErr.span.span.Meta["expected_exit_code"])

	// exit 0 with unrelated expected codes: still succeeds, no expected_exit_code tag
	cmdOk := CommandContext(ctx, "sh", "-c", "exit 0").WithExpectedExitCodes(5)
	require.NoError(t, cmdOk.Run())
	assert.Equal(t, int32(0), cmdOk.span.span.Error)
	assert.NotContains(t, cmdOk.span.span.Meta, "expected_exit_code")
}

func TestTracedCmdWithExpectedExitCodesAccumulates(t *testing.T) {
	ctx := context.Background()
	// Chained calls should accumulate: both 1 and 2 become expected
	cmd := CommandContext(ctx, "sh", "-c", "exit 2").
		WithExpectedExitCodes(1).
		WithExpectedExitCodes(2)
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, int32(0), cmd.span.span.Error)
	assert.Equal(t, float64(2), cmd.span.span.Metrics["exit_code"])
	assert.Equal(t, "true", cmd.span.span.Meta["expected_exit_code"])
}

func TestTracedCmdRunUnexpectedCodeAmongExpected(t *testing.T) {
	ctx := context.Background()
	// exit 2 is not in the expected set (only 1 and 5 are)
	cmd := CommandContext(ctx, "sh", "-c", "exit 2").WithExpectedExitCodes(1, 5)
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, int32(1), cmd.span.span.Error)
	assert.Equal(t, float64(2), cmd.span.span.Metrics["exit_code"])
	assert.NotContains(t, cmd.span.span.Meta, "expected_exit_code")
}
