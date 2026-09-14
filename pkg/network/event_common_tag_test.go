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
	require.False(t, conn.HasNStatTXRetransmitted())

	conn.AddTag(ConnTagTCPErrorsIncomplete)
	require.True(t, conn.HasTCPErrorsIncomplete())
	conn.AddTag(ConnTagTCPErrorsIncomplete)
	require.Len(t, conn.Tags, 1)

	conn.SetNStatTXRetransmittedHint(17)
	require.True(t, conn.HasNStatTXRetransmitted())
	require.True(t, conn.HasTag(ConnTagNStatTXRetransmitted))

	conn.SetNStatTXRetransmittedHint(4)
	require.True(t, conn.HasNStatTXRetransmitted())
	require.True(t, conn.HasTCPErrorsIncomplete())
	require.Len(t, conn.Tags, 2)

	conn.SetNStatTXRetransmittedHint(0)
	require.False(t, conn.HasNStatTXRetransmitted())
	require.True(t, conn.HasTCPErrorsIncomplete())

	conn.RemoveTag(ConnTagTCPErrorsIncomplete)
	require.False(t, conn.HasTCPErrorsIncomplete())
	require.Empty(t, conn.Tags)
}

func TestRemoveTagDoesNotAliasCopiedTags(t *testing.T) {
	var conn ConnectionStats
	conn.AddTag(ConnTagTCPErrorsIncomplete)
	conn.SetNStatTXRetransmittedHint(9)

	shallow := conn
	cloned := conn
	cloned.Tags = conn.CloneTags()

	conn.RemoveTag(ConnTagTCPErrorsIncomplete)
	conn.SetNStatTXRetransmittedHint(0)

	require.True(t, shallow.HasTCPErrorsIncomplete())
	require.True(t, shallow.HasNStatTXRetransmitted())
	require.True(t, cloned.HasTCPErrorsIncomplete())
	require.True(t, cloned.HasNStatTXRetransmitted())
	require.False(t, conn.HasTCPErrorsIncomplete())
	require.False(t, conn.HasNStatTXRetransmitted())
}

func TestRemoveTagMissingDoesNotAllocate(t *testing.T) {
	var conn ConnectionStats
	conn.AddTag(ConnTagTCPErrorsIncomplete)
	original := conn.Tags

	conn.RemoveTag(ConnTagNStatTXRetransmitted)
	conn.SetNStatTXRetransmittedHint(0)

	require.Equal(t, original, conn.Tags)
	require.Same(t, &original[0], &conn.Tags[0])
	require.True(t, conn.HasTCPErrorsIncomplete())
}
