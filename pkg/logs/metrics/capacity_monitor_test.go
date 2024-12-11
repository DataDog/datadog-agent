// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockPayload struct {
	count int64
	size  int64
}

func (m mockPayload) Size() int64 {
	return m.size
}
func (m mockPayload) Count() int64 {
	return m.count
}

func TestCapacityMonitor(t *testing.T) {

	tickChan := make(chan time.Time, 1)
	m := newCapacityMonitorWithTick("test", "test", tickChan)

	assert.Equal(t, m.avgItems, 0.0)
	assert.Equal(t, m.avgBytes, 0.0)

	// Tick before ingress - causing sample and flush.
	// Should converge on 10
	for i := 0; i < 60; i++ {
		tickChan <- time.Now()
		m.AddIngress(mockPayload{count: 10, size: 10})
		m.AddEgress(mockPayload{count: 10, size: 10})
	}
	assert.Greater(t, m.avgItems, 9.0)
	assert.Greater(t, m.avgBytes, 9.0)

	// Tick before egress - causing sample and flush.
	// Should converge on 0
	for i := 0; i < 60; i++ {
		m.AddIngress(mockPayload{count: 10, size: 10})
		tickChan <- time.Now()
		m.AddEgress(mockPayload{count: 10, size: 10})
	}

	assert.Less(t, m.avgItems, 1.0)
	assert.Less(t, m.avgBytes, 1.0)

}
