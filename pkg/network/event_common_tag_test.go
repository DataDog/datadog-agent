// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package network

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTCPErrorsIncompleteTagHelpers(t *testing.T) {
	var conn ConnectionStats
	require.False(t, conn.HasTCPErrorsIncomplete())

	conn.AddTag(ConnTagTCPErrorsIncomplete)
	require.True(t, conn.HasTCPErrorsIncomplete())
	require.True(t, conn.HasTag(ConnTagTCPErrorsIncomplete))
	conn.AddTag(ConnTagTCPErrorsIncomplete)
	require.Len(t, conn.Tags, 1)

	cloned := conn.CloneTags()
	require.Equal(t, conn.Tags, cloned)
	require.NotSame(t, &conn.Tags[0], &cloned[0])
}
