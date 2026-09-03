// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueueSizeTelemetryTracksJobs(t *testing.T) {
	// Use an interval no other test in this package uses so the gauge's label set is unique and
	// the assertions can't be perturbed by the process-wide telemetry registry.
	queue := newJobQueue(977*time.Second, false)
	t.Cleanup(func() { tlmQueueSize.Delete(queue.telemetryTags()...) })

	size := tlmQueueSize.WithValues(queue.telemetryTags()...)

	queue.addJob(&TestJobCheck{id: "first"})
	queue.addJob(&TestJobCheck{id: "second"})
	require.Equal(t, 2.0, size.Get())

	require.NoError(t, queue.removeJob("first"))
	require.Equal(t, 1.0, size.Get())

	// Removing a check that isn't queued must not move the gauge.
	require.Error(t, queue.removeJob("not-scheduled"))
	require.Equal(t, 1.0, size.Get())
}
