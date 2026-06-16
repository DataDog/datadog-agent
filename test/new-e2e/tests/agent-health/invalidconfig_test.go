// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
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

// invalidConfigAgentConfig sets agent_ipc.port to a string instead of an
// integer, triggering a schema validation error at agent startup.
// The forwarder interval is short to reduce detection latency in tests.
//
//go:embed fixtures/invalidconfig_agent_config.yaml
var invalidConfigAgentConfig string

// invalidConfigValidAgentConfig is a valid datadog.yaml used in the Resolution
// phase. It keeps the same short forwarder interval so the RESOLVED state
// reaches fakeintake quickly.
//
//go:embed fixtures/invalidconfig_valid_agent_config.yaml
var invalidConfigValidAgentConfig string

const (
	invalidConfigDetectionTimeout = 3 * time.Minute
	invalidConfigPollInterval     = 5 * time.Second
	invalidConfigAbsenceWindow    = 30 * time.Second
)

type invalidConfigSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestInvalidConfigSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &invalidConfigSuite{},
		e2e.WithDevMode(),
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(invalidConfigAgentConfig),
				),
			),
		)),
	)
}

// TestInvalidConfigIssueLifecycle verifies that a schema violation in
// datadog.yaml is detected in fakeintake as NEW at agent startup, and that
// redeploying the agent with a valid configuration causes the issue to
// transition to RESOLVED and stop being re-reported.
//
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *invalidConfigSuite) TestInvalidConfigIssueLifecycle() {
	fakeIntake := suite.Env().FakeIntake.Client()

	const issueID = "invalid-config"

	suite.T().Run("IssueDetection", func(t *testing.T) {
		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "invalid-config issue not found as NEW in fakeintake")
		}, invalidConfigDetectionTimeout, invalidConfigPollInterval, "invalid-config issue not detected in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.Equal(t, "invalid-config", issue.IssueName)
		assert.Equal(t, "configuration", issue.Category)
		assert.Equal(t, "config", issue.Source)
		assert.Equal(t, "agent", issue.Location)
		assert.Contains(t, issue.Tags, "config")
		assert.Contains(t, issue.Tags, "schema")
		require.NotNil(t, issue.Remediation)
		assert.NotEmpty(t, issue.Remediation.Summary)
		require.NotNil(t, issue.Extra)
		errorsVal := issue.Extra.GetFields()["errors"]
		require.NotNil(t, errorsVal, "extra must contain an 'errors' field")
		errorsStruct := errorsVal.GetStructValue()
		require.NotNil(t, errorsStruct, "extra.errors must be a struct keyed by config path")

		// agent_ipc.port is set to a non-integer in the test config, so a violation
		// must be reported under the /agent_ipc/port path.
		portVal := errorsStruct.GetFields()["/agent_ipc/port"]
		require.NotNil(t, portVal, "extra.errors must contain a /agent_ipc/port entry")
		portList := portVal.GetListValue()
		require.NotNil(t, portList, "extra.errors['/agent_ipc/port'] must be a list")
		require.NotEmpty(t, portList.GetValues(), "error list for /agent_ipc/port must be non-empty")
		assert.Contains(t, portList.GetValues()[0].GetStringValue(), "integer",
			"error message must describe the integer type mismatch")
	})

	suite.T().Run("Resolution", func(t *testing.T) {
		suite.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(invalidConfigValidAgentConfig),
				),
			),
		))

		// Wait for the issue to explicitly transition to RESOLVED before flushing.
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "issue not yet RESOLVED")
		}, invalidConfigDetectionTimeout, invalidConfigPollInterval,
			"issue never transitioned to RESOLVED after deploying valid config")

		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

		require.Never(t, func() bool {
			payloads, _ := fakeIntake.GetAgentHealth()
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, issueID) {
					if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return true
					}
				}
			}
			return false
		}, invalidConfigAbsenceWindow, invalidConfigPollInterval,
			"invalid-config issue reappeared as non-resolved after flush")
	})
}
