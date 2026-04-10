// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localprocessingonly tests that logs matching a local_processing_only rule
// are not forwarded to the Datadog backend (fakeintake), while unmatched logs still arrive.
package localprocessingonly

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

//go:embed config/agent_config.yaml
var agentConfig string

//go:embed config/suppressed-svc.yaml
var suppressedSvcConfig string

//go:embed config/allowed-svc.yaml
var allowedSvcConfig string

const (
	suppressedLogFile = "suppressed-svc.log"
	allowedLogFile    = "allowed-svc.log"
)

// LocalProcessingOnlySuite verifies the local_processing_only routing rule.
type LocalProcessingOnlySuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestLocalProcessingOnlySuite is the entry point for the E2E test suite.
func TestLocalProcessingOnlySuite(t *testing.T) {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(
						agentparams.WithLogs(),
						agentparams.WithAgentConfig(agentConfig),
						agentparams.WithIntegration("suppressed_svc_logs.d", suppressedSvcConfig),
						agentparams.WithIntegration("allowed_svc_logs.d", allowedSvcConfig),
					)))),
	}
	t.Parallel()
	e2e.Run(t, &LocalProcessingOnlySuite{}, options...)
}

func (s *LocalProcessingOnlySuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	utils.CleanUp(s)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("suppressed-svc")
		require.NoError(c, err)
		assert.Empty(c, logs, "Found unexpected logs for suppressed-svc before test started")
	}, 2*time.Minute, 10*time.Second)

	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + utils.LinuxLogsFolderPath)
}

func (s *LocalProcessingOnlySuite) TearDownSuite() {
	utils.CleanUp(s)
	s.BaseSuite.TearDownSuite()
}

// TestLocalProcessingOnlySuppressionByService verifies that:
//   - Logs from "suppressed-svc" do NOT arrive at fakeintake.
//   - Logs from "allowed-svc" DO arrive at fakeintake.
func (s *LocalProcessingOnlySuite) TestLocalProcessingOnlySuppressionByService() {
	// Generate logs for both services.
	utils.AppendLog(s, allowedLogFile, "allowed-log-content", 1)
	utils.AppendLog(s, suppressedLogFile, "suppressed-log-content", 1)

	// First, wait until the allowed-svc logs arrive so we know the agent has
	// processed the batch — then verify suppressed-svc logs are absent.
	utils.CheckLogsExpected(s.T(), s.Env().FakeIntake, "allowed-svc", "allowed-log-content", []string{})

	// Verify that suppressed-svc logs never reach fakeintake.
	// We use a short fixed poll since we already confirmed the agent is processing.
	s.T().Log("Verifying suppressed-svc logs are absent from fakeintake")
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("suppressed-svc")
		assert.NoError(c, err, "Error querying fakeintake for suppressed-svc")
		assert.Empty(c, logs, "suppressed-svc logs must not reach fakeintake")
	}, 30*time.Second, 5*time.Second)
}
