// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package foldspace is an ALP suite that exercises foldspace dual-ship and
// foldspace-only delivery against fakeintake.
package foldspace

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

//go:embed config/config.yaml
var logConfig string

const foldspaceDualShipConfig = `
logs_config:
  foldspace:
    enabled: true
    dual_ship: true
`

const foldspaceOnlyConfig = `
logs_config:
  foldspace:
    enabled: true
    dual_ship: false
`

type dualShipSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestFoldspaceDualShip(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dualShipSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithLogs(),
					agentparams.WithAgentConfig(foldspaceDualShipConfig),
					agentparams.WithIntegration("custom_logs.d", logConfig),
				)))),
	)
}

func (s *dualShipSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	utils.CleanUp(s)
}

func (s *dualShipSuite) TestLogCollection() {
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + utils.LinuxLogsFolderPath)
	utils.AppendLog(s, "hello-world.log", "hello-world", 1)
	utils.CheckLogsExpected(s.T(), s.Env().FakeIntake, "hello", "hello-world", []string{})
}

type foldspaceOnlySuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestFoldspaceOnly(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &foldspaceOnlySuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithLogs(),
					agentparams.WithAgentConfig(foldspaceOnlyConfig),
					agentparams.WithIntegration("custom_logs.d", logConfig),
				)))),
	)
}

func (s *foldspaceOnlySuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	utils.CleanUp(s)
}

func (s *foldspaceOnlySuite) TestFilterLogsOnStatefulPath() {
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + utils.LinuxLogsFolderPath)
	utils.AppendLog(s, "hello-world.log", "hello-world", 1)
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("hello")
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, logs)
	}, utils.WaitFor, utils.Tick)
}
