// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearLogsHealth(t *testing.T) {
	t.Cleanup(ResetDroppedLogsForTest)
	t.Cleanup(ResetMissedBytesForTest)
	MarkLogsAgentRunning()
	RecordDroppedLogs(DropReasonAuth, DropTarget{Host: "intake", Main: true}, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10},
	})
	RecordMissedBytes("nginx", "web", 10)
	require.NotEmpty(t, DroppedLogsSnapshot())
	require.NotEmpty(t, MissedBytesSnapshot())

	ClearLogsHealth()

	assert.Empty(t, DroppedLogsSnapshot())
	assert.Empty(t, MissedBytesSnapshot())
	assert.True(t, LogsAgentRunning(), "the checks must still see a running logs agent, or they would not resolve the issues")
}
