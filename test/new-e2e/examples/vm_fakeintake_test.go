// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"errors"
	"time"

	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
	ec2vm "github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/os"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vmFakeintakeSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, e2e.AgentStackDef([]e2e.Ec2VMOption{ec2vm.WithArch(os.UbuntuOS, commonos.AMD64Arch)}))
}

func (s *vmFakeintakeSuite) TestVM() {
	output := s.Env().VM.Execute("ls")
	require.NotEmpty(s.T(), output)
}

func (s *vmFakeintakeSuite) TestAgent() {
	err := s.Env().Agent.WaitForReady()
	require.NoError(s.T(), err)
	output := s.Env().Agent.Status()
	require.Contains(s.T(), output.Content, "Getting the status from the agent")
	isReady, err := s.Env().Agent.IsReady()
	require.NoError(s.T(), err)
	assert.True(s.T(), isReady, "Agent is not ready")
}

func (s *vmFakeintakeSuite) TestMetrics() {
	t := s.T()
	err := backoff.Retry(func() error {
		metrics, err := s.Env().Fakeintake.Client.GetMetric("system.uptime")
		if err != nil {
			return err
		}
		if len(metrics) == 0 {
			return errors.New("No metrics yet")
		}
		if metrics[len(metrics)-1].Points[len(metrics[len(metrics)-1].Points)-1].Value == 0 {
			return errors.New("")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 20))
	require.NoError(t, err)
}

func (s *vmFakeintakeSuite) TestCheckRuns() {
	t := s.T()
	err := backoff.Retry(func() error {
		checkRuns, err := s.Env().Fakeintake.Client.GetCheckRun("datadog.agent.up")
		if err != nil {
			return err
		}
		if len(checkRuns) == 0 {
			return errors.New("No check run yet")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 20))
	require.NoError(t, err)
}
