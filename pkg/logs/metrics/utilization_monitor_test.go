// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
)

func TestUtilizationMonitorLifecycle(t *testing.T) {
	clock := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("name", "instance", 2*time.Second, clock)

	// Converge on 50% utilization
	for i := 0; i < 100; i++ {
		um.Start()
		clock.Add(1 * time.Second)

		um.Stop()
		clock.Add(1 * time.Second)
	}

	require.InDelta(t, 0.5, um.avg, 0.01)

	// Converge on 100% utilization
	for i := 0; i < 100; i++ {
		um.Start()
		clock.Add(1 * time.Second)

		um.Stop()
		clock.Add(1 * time.Millisecond)
	}

	require.InDelta(t, 0.99, um.avg, 0.01)

	// Converge on 0% utilization
	for i := 0; i < 200; i++ {
		um.Start()
		clock.Add(1 * time.Millisecond)

		um.Stop()
		clock.Add(1 * time.Second)
	}

	require.InDelta(t, 0.0, um.avg, 0.01)

}
