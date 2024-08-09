// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCheck(t *testing.T) {
	telemetryMock := fxutil.Test[telemetryComponent.Mock](t, telemetryimpl.MockModule())
	telemetryMock.Reset()

	func() {
		gauge := telemetryMock.NewGaugeWithOpts("test", "gauge", []string{"foo"}, "", telemetryComponent.Options{
			DefaultMetric: true,
		})

		gauge.WithTags(map[string]string{"foo": "bar"}).Set(1.0)
		gauge.WithTags(map[string]string{"foo": "baz"}).Set(2.0)

		count := telemetryMock.NewCounterWithOpts("test", "counter", []string{}, "", telemetryComponent.Options{
			DefaultMetric: true,
		})
		count.Add(4.0)
	}()

	sm := mocksender.CreateDefaultDemultiplexer()

	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName), telemetry: telemetryMock}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test")

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Commit").Return().Times(1)

	mfs, err := telemetryMock.Gather(true)
	require.Nil(t, err)

	c.handleMetricFamilies(mfs, s)
	s.AssertExpectations(t)
}
