// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

type vmSuiteEx6 struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestVMSuiteEx6(t *testing.T) {
	e2e.Run(t, &vmSuiteEx6{}, e2e.FakeIntakeStackDef(nil))
}

func (v *vmSuiteEx6) Test1_FakeIntakeReceivesMetrics() {
	t := v.T()
	err := backoff.Retry(func() error {
		metricNames, err := v.Env().Fakeintake.GetConnections()
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
