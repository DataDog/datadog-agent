// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	_ "embed"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/win32_eventlog_channels_agent_config.yaml
var win32EventLogChannelsAgentConfig string

// invalidChannelConf is the win32_event_log config with a non-existent channel.
const invalidChannelConf = `instances:
  - path: NonExistentChannel123
    legacy_mode: false
`

// validChannelConf is the win32_event_log config with a valid channel.
const validChannelConf = `instances:
  - path: System
    legacy_mode: false
`

const win32EventLogCheckConfPath = `C:\ProgramData\Datadog\conf.d\win32_event_log.d\conf.yaml`
const win32HealthPlatformIssuesPath = `C:\ProgramData\Datadog\run\health-platform\issues.json`

type win32EventLogChannelsEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

func win32EventLogChannelsEnvProvisioner() provisioners.PulumiEnvRunFunc[win32EventLogChannelsEnv] {
	return func(ctx *pulumi.Context, env *win32EventLogChannelsEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "win32evtlogvm", ec2.WithOS(ec2.WindowsDefault))
		if err != nil {
			return err
		}
		if err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		// Skip forwarding to dddev — agenthealth intake is only on staging
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
		if err != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
			return err
		}

		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithAgentConfig(win32EventLogChannelsAgentConfig),
		)
		if err != nil {
			return err
		}
		if err = hostAgent.Export(ctx, &env.Agent.HostAgentOutput); err != nil {
			return err
		}

		return nil
	}
}

type win32EventLogChannelsSuite struct {
	e2e.BaseSuite[win32EventLogChannelsEnv]
}

// TestWin32EventLogChannelsSuite runs the Windows Event Log channel health check test.
func TestWin32EventLogChannelsSuite(t *testing.T) {
	e2e.Run(t, &win32EventLogChannelsSuite{},
		e2e.WithPulumiProvisioner(win32EventLogChannelsEnvProvisioner(), nil),
	)
}

// TestWin32EventLogChannelNotFoundDiagnose verifies the detect-and-remediate flow for the
// windows-eventlog-channel-not-found health check:
//  1. With an invalid channel_path in the win32_event_log config, agent diagnose reports
//     the windows-eventlog-channel-not-found issue with a WARNING.
//  2. After fixing the config to a valid channel and restarting, the issue is gone.
//
// The check reports the issue inline in Run() when EvtSubscribe returns
// ERROR_EVT_CHANNEL_NOT_FOUND. The check requires legacy_mode: false to use the Go
// implementation; the legacy Python check does not call EvtSubscribe.
func (suite *win32EventLogChannelsSuite) TestWin32EventLogChannelNotFoundDiagnose() {
	t := suite.T()
	host := suite.Env().RemoteHost
	agentComp := suite.Env().Agent

	// Restore a clean config after the test.
	t.Cleanup(func() {
		host.Execute(`Remove-Item -Force '` + win32EventLogCheckConfPath + `' -ErrorAction SilentlyContinue`) //nolint:errcheck
		host.Execute(`Remove-Item -Force '` + win32HealthPlatformIssuesPath + `' -ErrorAction SilentlyContinue`) //nolint:errcheck
		_ = agentComp.Client.Restart()
	})

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready")
	}, 3*time.Minute, 10*time.Second, "agent not ready")

	// -------------------------------------------------------------------------
	// Step 1: Configure an invalid channel and restart to trigger the check
	// -------------------------------------------------------------------------

	host.MustExecute(`Set-Content '` + win32EventLogCheckConfPath + `' -Value @'
` + invalidChannelConf + `
'@ -Encoding UTF8`)

	host.MustExecute(`Remove-Item -Force '` + win32HealthPlatformIssuesPath + `' -ErrorAction SilentlyContinue`)

	require.NoError(t, agentComp.Client.Restart(), "failed to restart agent after writing invalid channel config")
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready after restart")
	}, 3*time.Minute, 10*time.Second, "agent not ready after restart")

	diagnoseOut := host.MustExecute(`& 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' diagnose --verbose`)
	assert.Contains(t, diagnoseOut, "windows-eventlog-channel-not-found", "diagnose should report windows-eventlog-channel-not-found")
	assert.Contains(t, diagnoseOut, "WARNING", "issue should be reported as WARNING")
	assert.Contains(t, diagnoseOut, "NonExistentChannel123", "diagnosis should name the offending channel")
	t.Log("Step 1 passed: windows-eventlog-channel-not-found issue detected by diagnose")

	// -------------------------------------------------------------------------
	// Step 2: Fix the config and restart to verify resolution
	// -------------------------------------------------------------------------

	host.MustExecute(`Set-Content '` + win32EventLogCheckConfPath + `' -Value @'
` + validChannelConf + `
'@ -Encoding UTF8`)

	host.MustExecute(`Remove-Item -Force '` + win32HealthPlatformIssuesPath + `' -ErrorAction SilentlyContinue`)

	require.NoError(t, agentComp.Client.Restart(), "failed to restart agent after fixing config")
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready after remediation restart")
	}, 3*time.Minute, 10*time.Second, "agent not ready after remediation restart")

	diagnoseOut = host.MustExecute(`& 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' diagnose --verbose`)
	assert.NotContains(t, diagnoseOut, "windows-eventlog-channel-not-found", "diagnose should not report the issue after fixing the channel")
	t.Log("Step 2 passed: windows-eventlog-channel-not-found issue resolved after fixing config")
}
