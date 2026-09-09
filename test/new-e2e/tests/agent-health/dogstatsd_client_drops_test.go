// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/dogstatsd_client_telemetry.py
var dogstatsdClientTelemetryScript string

const (
	dogstatsdClientDropsIssueIDPrefix = "dogstatsd-go-uds-client-payload-drops:"
	dogstatsdSocketPath               = "/var/run/datadog/dsd.socket"
	dogstatsdClientTelemetryPath      = "/tmp/dogstatsd_client_telemetry.py"
	dogstatsdClientTelemetryPIDPath   = "/tmp/dogstatsd_client_telemetry.pid"
	dogstatsdClientTelemetryLogPath   = "/tmp/dogstatsd_client_telemetry.log"
	dogstatsdClientUnhealthyMode      = "unhealthy"
	dogstatsdClientHealthyMode        = "healthy"
	dogstatsdClientDropsAgentConfig   = `
health_platform:
  enabled: true
  forwarder:
    interval: 5s
dogstatsd_socket: /var/run/datadog/dsd.socket
dogstatsd_client_drop_detection:
  enabled: true
  unhealthy_confirmation_window: 1s
  recovery_confirmation_window: 1s
`
)

type dogstatsdClientDropsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestDogStatsDClientDropsSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogstatsdClientDropsSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(dogstatsdClientDropsAgentConfig),
					agentparams.WithFile(dogstatsdClientTelemetryPath, dogstatsdClientTelemetryScript, false),
				),
			),
		)),
	)
}

func (suite *dogstatsdClientDropsSuite) TestDogStatsDClientDropsIssueLifecycle() {
	fakeIntake := suite.Env().FakeIntake.Client()
	suite.T().Cleanup(suite.stopDogStatsDClient)

	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())
	suite.startDogStatsDClient(dogstatsdClientUnhealthyMode)

	var detectedIssue *healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		detectedIssue = nil
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		for _, payload := range payloads {
			for _, issue := range findIssuesByPrefix(payload, dogstatsdClientDropsIssueIDPrefix) {
				if issue.PersistedIssue != nil && issue.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
					detectedIssue = issue
					return
				}
			}
		}
		assert.Fail(ct, "DogStatsD client drops issue not found as ACTIVE in fakeintake")
	}, defaultIssueTimeout, defaultIssuePollInterval, "DogStatsD client drops issue not detected in fakeintake")

	require.NotNil(suite.T(), detectedIssue)
	detectedIssueID := detectedIssue.GetId()
	require.NotEmpty(suite.T(), detectedIssueID)
	assert.Equal(suite.T(), "DogStatsD Go UDS Client Payload Drops", detectedIssue.IssueName)
	assert.Equal(suite.T(), "dogstatsd_go_uds_client_payload_drops", detectedIssue.IssueType)
	assert.Equal(suite.T(), "dogstatsd", detectedIssue.Category)
	assert.Equal(suite.T(), "dogstatsd", detectedIssue.Location)
	assert.Equal(suite.T(), "dogstatsd", detectedIssue.Source)
	assert.Equal(suite.T(), healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, detectedIssue.Severity)
	assert.Contains(suite.T(), detectedIssue.Tags, "client:go")
	assert.Contains(suite.T(), detectedIssue.Tags, "uds")
	require.NotNil(suite.T(), detectedIssue.Remediation)
	assert.NotEmpty(suite.T(), detectedIssue.Remediation.Steps)

	suite.stopDogStatsDClient()
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())
	suite.startDogStatsDClient(dogstatsdClientHealthyMode)

	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		for _, payload := range payloads {
			for _, issue := range findIssuesByID(suite.T(), payload, detectedIssueID) {
				if issue.PersistedIssue != nil && issue.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
					return
				}
			}
		}
		assert.Fail(ct, "DogStatsD client drops issue not found as RESOLVED in fakeintake")
	}, defaultIssueTimeout, defaultIssuePollInterval, "DogStatsD client drops issue never transitioned to RESOLVED")
}

func (suite *dogstatsdClientDropsSuite) startDogStatsDClient(mode string) {
	host := suite.Env().RemoteHost
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		host.MustExecuteOn(ct, "test -S "+dogstatsdSocketPath)
	}, time.Minute, 5*time.Second, "DogStatsD socket was not ready")

	command := fmt.Sprintf(
		"nohup env DOGSTATSD_SOCKET=%s TELEMETRY_MODE=%s /opt/datadog-agent/embedded/bin/python %s >%s 2>&1 </dev/null & echo -n $! > %s",
		dogstatsdSocketPath,
		mode,
		dogstatsdClientTelemetryPath,
		dogstatsdClientTelemetryLogPath,
		dogstatsdClientTelemetryPIDPath,
	)
	host.MustExecute(command)
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		host.MustExecuteOn(ct, "kill -0 $(cat "+dogstatsdClientTelemetryPIDPath+")")
	}, time.Minute, time.Second, "DogStatsD client workload did not start")
}

func (suite *dogstatsdClientDropsSuite) stopDogStatsDClient() {
	_, err := suite.Env().RemoteHost.Execute(
		"if test -f " + dogstatsdClientTelemetryPIDPath + "; then " +
			"kill $(cat " + dogstatsdClientTelemetryPIDPath + ") 2>/dev/null || true; " +
			"rm -f " + dogstatsdClientTelemetryPIDPath + "; " +
			"fi",
	)
	require.NoError(suite.T(), err)
}
