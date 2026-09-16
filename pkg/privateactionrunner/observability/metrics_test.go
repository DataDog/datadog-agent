// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportHealthCheck(t *testing.T) {
	recorder := &recordingTaggedStatsdClient{}

	ReportHealthCheck(recorder)

	require.Len(t, recorder.calls, 2)
	assert.Equal(t, RunnerRunningMetric, recorder.calls[0].name)
	assert.Equal(t, HealthCheckMetric, recorder.calls[1].name)
}

func TestReportHealthCheck_NilMetricsClientDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		ReportHealthCheck(nil)
	})
}
