// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

func TestClearHealthEndpoint(t *testing.T) {
	t.Cleanup(metrics.ResetDroppedLogsForTest)
	metrics.RecordDroppedLogs(metrics.DropReasonAuth, metrics.DropTarget{Host: "intake", Main: true},
		map[metrics.DropOrigin]metrics.DropCount{{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10}})
	require.NotEmpty(t, metrics.DroppedLogsSnapshot())

	rec := httptest.NewRecorder()
	clearHealth(rec, httptest.NewRequest(http.MethodPost, "/logs/clear-health", nil))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, metrics.DroppedLogsSnapshot())
}
