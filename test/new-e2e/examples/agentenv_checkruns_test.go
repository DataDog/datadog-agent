// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

type agentSuiteEx5 struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestAgentSuiteEx5(t *testing.T) {
	e2e.Run(t, &agentSuiteEx5{}, e2e.FakeIntakeStackDef(nil))
}

func (s *agentSuiteEx5) TestCheckRuns() {
	t := s.T()
	err := backoff.Retry(func() error {
		checkRuns, err := s.Env().Fakeintake.Client.GetCheckRun("datadog.agent.up")
		if err != nil {
			return err
		}
		if len(checkRuns) == 0 {
			return errors.New("no check run yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	require.NoError(t, err)
}
