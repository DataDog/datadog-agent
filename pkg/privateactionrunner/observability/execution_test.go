// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

func TestExecutionDetails(t *testing.T) {
	ctx, first := WithExecutionDetails(context.Background())
	_, second := WithExecutionDetails(context.Background())
	fields := []log.Field{log.String("version", "1.2.3")}
	SetExecutionDetails(context.WithoutCancel(ctx), "execution", fields...)
	fields[0] = log.String("version", "mutated")
	stage, got, provided := first.snapshot()
	require.True(t, provided)
	require.Equal(t, "execution", stage)
	require.Equal(t, []log.Field{log.String("version", "1.2.3")}, got)
	got[0] = log.String("version", "mutated snapshot")
	_, got, _ = first.snapshot()
	require.Equal(t, "1.2.3", got[0].Value)
	_, _, provided = second.snapshot()
	require.False(t, provided, "metadata must not leak to another execution")
	SetExecutionDetails(context.Background(), "execution", fields...) // Direct action calls have no collector.
}
