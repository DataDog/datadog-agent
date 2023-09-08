// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

type vmSuiteEx5 struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestVMSuiteEx5(t *testing.T) {
	e2e.Run(t, &vmSuiteEx5{}, e2e.FakeIntakeStackDef(nil))
}

func (v *vmSuiteEx5) Test1_FakeIntakeReceivesMetrics() {
	t := v.T()
	err := backoff.Retry(func() error {
		metricNames, err := v.Env().Fakeintake.GetMetricNames()
		if err != nil {
			return err
		}
		if len(metricNames) == 0 {
			return errors.New("no metrics yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}

func (v *vmSuiteEx5) Test2_FakeIntakeReceivesSystemLoadMetric() {
	t := v.T()
	err := backoff.Retry(func() error {
		metrics, err := v.Env().Fakeintake.FilterMetrics("system.load.1")
		if err != nil {
			return err
		}
		if len(metrics) == 0 {
			return errors.New("no 'system.load.1' metrics yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}

func (v *vmSuiteEx5) Test3_FakeIntakeReceivesSystemUptimeHigherThanZero() {
	t := v.T()
	err := backoff.Retry(func() error {
		metrics, err := v.Env().Fakeintake.FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		if err != nil {
			return err
		}
		if len(metrics) == 0 {
			return errors.New("no 'system.uptime' with value higher than 0 yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}
