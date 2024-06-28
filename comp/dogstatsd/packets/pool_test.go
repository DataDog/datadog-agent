// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPoolTelemetry(t *testing.T) {
	usedByTestTelemetry = true
	defer func() {
		usedByTestTelemetry = false
	}()

	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(1024, packetsTelemetryStore)

	packet := &Packet{
		Contents:   []byte("test"),
		Buffer:     []byte("test read"),
		Origin:     "test origin",
		ListenerID: "1",
		Source:     0,
	}

	pool.Put(packet)

	telemetryMock, ok := telemetryComponent.(telemetry.Mock)
	assert.True(t, ok)

	poolMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packet_pool")
	assert.Nil(t, err)
	pollPutMetrics, err := telemetryMock.GetCountMetric("dogstatsd", "packet_pool_put")
	assert.Nil(t, err)

	assert.Equal(t, float64(-1), poolMetrics[0].Value())
	assert.Equal(t, float64(1), pollPutMetrics[0].Value())

	pool.Get()

	poolMetrics, err = telemetryMock.GetGaugeMetric("dogstatsd", "packet_pool")
	assert.Nil(t, err)
	pollGetMetrics, err := telemetryMock.GetCountMetric("dogstatsd", "packet_pool_get")
	assert.Nil(t, err)

	assert.Equal(t, float64(0), poolMetrics[0].Value())
	assert.Equal(t, float64(1), pollGetMetrics[0].Value())

}
