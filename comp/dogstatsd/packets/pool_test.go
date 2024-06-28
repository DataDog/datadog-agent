// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
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

	var poolMetric []*dto.Metric
	var pollPutMetric []*dto.Metric
	var pollGetMetric []*dto.Metric

	metricsFamily, err := telemetryMock.GetRegistry().Gather()
	assert.Nil(t, err)

	for _, metric := range metricsFamily {
		if metric.GetName() == "dogstatsd__packet_pool" {
			poolMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packet_pool_put" {
			pollPutMetric = metric.GetMetric()
		}
	}

	assert.NotNil(t, poolMetric)
	assert.NotNil(t, pollPutMetric)

	assert.Equal(t, float64(-1), poolMetric[0].GetGauge().GetValue())
	assert.Equal(t, float64(1), pollPutMetric[0].GetCounter().GetValue())

	pool.Get()

	metricsFamily, err = telemetryMock.GetRegistry().Gather()
	assert.Nil(t, err)

	for _, metric := range metricsFamily {
		if metric.GetName() == "dogstatsd__packet_pool" {
			poolMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packet_pool_get" {
			pollGetMetric = metric.GetMetric()
		}
	}

	assert.NotNil(t, pollGetMetric)
	assert.NotNil(t, poolMetric)

	assert.Equal(t, float64(0), poolMetric[0].GetGauge().GetValue())
	assert.Equal(t, float64(1), pollGetMetric[0].GetCounter().GetValue())

}
