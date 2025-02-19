// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPipelineMonitorTracksCorrectCapacity(t *testing.T) {
	pm := NewTelemetryPipelineMonitor("test")

	pm.ReportComponentIngress(mockPayload{count: 1, size: 1}, "1")
	pm.ReportComponentIngress(mockPayload{count: 5, size: 5}, "5")
	pm.ReportComponentIngress(mockPayload{count: 10, size: 10}, "10")

	assert.Equal(t, pm.getMonitor("1").ingress, int64(1))
	assert.Equal(t, pm.getMonitor("1").ingressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5").ingress, int64(5))
	assert.Equal(t, pm.getMonitor("5").ingressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10").ingress, int64(10))
	assert.Equal(t, pm.getMonitor("10").ingressBytes, int64(10))

	pm.ReportComponentEgress(mockPayload{count: 1, size: 1}, "1")
	pm.ReportComponentEgress(mockPayload{count: 5, size: 5}, "5")
	pm.ReportComponentEgress(mockPayload{count: 10, size: 10}, "10")

	assert.Equal(t, pm.getMonitor("1").egress, int64(1))
	assert.Equal(t, pm.getMonitor("1").egressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5").egress, int64(5))
	assert.Equal(t, pm.getMonitor("5").egressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10").egress, int64(10))
	assert.Equal(t, pm.getMonitor("10").egressBytes, int64(10))

	assert.Equal(t, pm.getMonitor("1").ingress-pm.getMonitor("1").egress, int64(0))
	assert.Equal(t, pm.getMonitor("5").ingress-pm.getMonitor("5").egress, int64(0))
	assert.Equal(t, pm.getMonitor("10").ingress-pm.getMonitor("10").egress, int64(0))
}
