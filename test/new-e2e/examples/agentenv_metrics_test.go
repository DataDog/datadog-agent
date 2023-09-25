// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeintakeSuiteMetrics struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestVMSuiteEx5(t *testing.T) {
	e2e.Run(t, &fakeintakeSuiteMetrics{}, e2e.FakeIntakeStackDef())
}

func (v *fakeintakeSuiteMetrics) Test1_FakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().Fakeintake.GetMetricNames()
		require.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}

func (v *fakeintakeSuiteMetrics) Test2_FakeIntakeReceivesSystemLoadMetric() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().Fakeintake.FilterMetrics("system.load.1")
		require.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

func (v *fakeintakeSuiteMetrics) Test3_FakeIntakeReceivesSystemUptimeHigherThanZero() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().Fakeintake.FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		require.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.uptime' with value higher than 0 yet")
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}
