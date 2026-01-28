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
	pm := NewTelemetryPipelineMonitor()

	pm.ReportComponentIngress(mockPayload{count: 1, size: 1}, "1", "test")
	pm.ReportComponentIngress(mockPayload{count: 5, size: 5}, "5", "test")
	pm.ReportComponentIngress(mockPayload{count: 10, size: 10}, "10", "test")

	assert.Equal(t, pm.getMonitor("1", "test").ingress, int64(1))
	assert.Equal(t, pm.getMonitor("1", "test").ingressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5", "test").ingress, int64(5))
	assert.Equal(t, pm.getMonitor("5", "test").ingressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10", "test").ingress, int64(10))
	assert.Equal(t, pm.getMonitor("10", "test").ingressBytes, int64(10))

	pm.ReportComponentEgress(mockPayload{count: 1, size: 1}, "1", "test")
	pm.ReportComponentEgress(mockPayload{count: 5, size: 5}, "5", "test")
	pm.ReportComponentEgress(mockPayload{count: 10, size: 10}, "10", "test")

	assert.Equal(t, pm.getMonitor("1", "test").egress, int64(1))
	assert.Equal(t, pm.getMonitor("1", "test").egressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5", "test").egress, int64(5))
	assert.Equal(t, pm.getMonitor("5", "test").egressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10", "test").egress, int64(10))
	assert.Equal(t, pm.getMonitor("10", "test").egressBytes, int64(10))

	assert.Equal(t, pm.getMonitor("1", "test").ingress-pm.getMonitor("1", "test").egress, int64(0))
	assert.Equal(t, pm.getMonitor("5", "test").ingress-pm.getMonitor("5", "test").egress, int64(0))
	assert.Equal(t, pm.getMonitor("10", "test").ingress-pm.getMonitor("10", "test").egress, int64(0))
}
