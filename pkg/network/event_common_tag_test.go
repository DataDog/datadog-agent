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
	conn.AddTag(ConnTagTCPErrorsIncomplete)
	require.Len(t, conn.Tags, 1)

	conn.SetNStatTXRetransmittedBytesHint(17)
	hint, ok := conn.NStatTXRetransmittedBytesHint()
	require.True(t, ok)
	require.Equal(t, uint32(17), hint)

	conn.SetNStatTXRetransmittedBytesHint(4)
	hint, ok = conn.NStatTXRetransmittedBytesHint()
	require.True(t, ok)
	require.Equal(t, uint32(4), hint)
	require.True(t, conn.HasTCPErrorsIncomplete())

	conn.SetNStatTXRetransmittedBytesHint(0)
	_, ok = conn.NStatTXRetransmittedBytesHint()
	require.False(t, ok)
	require.True(t, conn.HasTCPErrorsIncomplete())

	conn.RemoveTag(ConnTagTCPErrorsIncomplete)
	require.False(t, conn.HasTCPErrorsIncomplete())
	require.Empty(t, conn.Tags)
}
