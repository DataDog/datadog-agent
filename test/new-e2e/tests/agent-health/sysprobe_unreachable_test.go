// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

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

//go:embed fixtures/sysprobe_unreachable_agent_config.yaml
var sysprobeUnreachableAgentConfig string

type sysprobeUnreachableEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

func sysprobeUnreachableEnvProvisioner() provisioners.PulumiEnvRunFunc[sysprobeUnreachableEnv] {
	return func(ctx *pulumi.Context, env *sysprobeUnreachableEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "sysprobevm")
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
			agentparams.WithAgentConfig(sysprobeUnreachableAgentConfig),
			// network_config and sysprobe_socket must be in system-probe.yaml;
			// the startup check reads them via pkgconfigsetup.SystemProbe().
			agentparams.WithSystemProbeConfig("network_config:\n  enabled: true\nsystem_probe_config:\n  sysprobe_socket: /opt/datadog-agent/run/sysprobe.sock"),
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

type sysprobeUnreachableSuite struct {
	e2e.BaseSuite[sysprobeUnreachableEnv]
}

// TestSysprobeUnreachableSuite runs the system-probe unreachable health check test.
func TestSysprobeUnreachableSuite(t *testing.T) {
	e2e.Run(t, &sysprobeUnreachableSuite{},
		e2e.WithPulumiProvisioner(sysprobeUnreachableEnvProvisioner(), nil),
	)
}

// TestSysprobeUnreachableDiagnose verifies the detect-and-remediate flow for the
// system-probe-unreachable health check:
//  1. With NPM enabled and system-probe stopped, agent diagnose reports the issue.
//  2. After starting system-probe and restarting the agent, the issue is gone.
//
// The check is a BuiltInStartupHealthCheck: it dials the sysprobe socket once at
// agent startup. Resolution therefore requires restarting the agent with system-probe
// already running and its socket accessible.
func (suite *sysprobeUnreachableSuite) TestSysprobeUnreachableDiagnose() {
	t := suite.T()
	host := suite.Env().RemoteHost
	agentComp := suite.Env().Agent

	// Restore system-probe after the test so infrastructure is ready for re-use.
	t.Cleanup(func() {
		host.Execute("sudo systemctl start datadog-agent-sysprobe") //nolint:errcheck
		_ = agentComp.Client.Restart()
	})

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready")
	}, 2*time.Minute, 10*time.Second, "agent not ready")

	// -------------------------------------------------------------------------
	// Step 1: Stop system-probe and restart agent to trigger the startup check
	// -------------------------------------------------------------------------

	host.MustExecute("sudo systemctl stop datadog-agent-sysprobe")
	t.Log("system-probe stopped")

	// Clear any persisted issues so the diagnose reflects only fresh detections.
	host.MustExecute("sudo rm -f /opt/datadog-agent/run/health-platform/issues.json")

	require.NoError(t, agentComp.Client.Restart(), "failed to restart agent after stopping system-probe")
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready after restart")
	}, 2*time.Minute, 10*time.Second, "agent not ready after restart")

	diagnoseOut := host.MustExecute("sudo datadog-agent diagnose --verbose")
	assert.Contains(t, diagnoseOut, "system-probe-unreachable", "diagnose should report system-probe-unreachable when system-probe is down")
	assert.Contains(t, diagnoseOut, "WARNING", "issue should be reported as WARNING")
	assert.Contains(t, diagnoseOut, "system-probe", "diagnosis should mention system-probe")
	t.Log("Step 1 passed: system-probe-unreachable issue detected by diagnose")

	// -------------------------------------------------------------------------
	// Step 2: Apply the remediation — start system-probe, then restart agent
	// -------------------------------------------------------------------------

	// Start system-probe and wait for its socket to be available before restarting
	// the agent, so the startup check finds an accessible socket.
	host.MustExecute("sudo systemctl start datadog-agent-sysprobe")
	t.Log("system-probe started (remediation applied)")

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		_, err := host.Execute("test -S /opt/datadog-agent/run/sysprobe.sock")
		assert.NoError(ct, err, "sysprobe socket should be available")
	}, 30*time.Second, 2*time.Second, "sysprobe socket did not appear")
	t.Log("sysprobe socket is ready")

	host.MustExecute("sudo rm -f /opt/datadog-agent/run/health-platform/issues.json")

	require.NoError(t, agentComp.Client.Restart(), "failed to restart agent after starting system-probe")
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.True(ct, agentComp.Client.IsReady(), "agent should be ready after remediation restart")
	}, 2*time.Minute, 10*time.Second, "agent not ready after remediation restart")

	diagnoseOut = host.MustExecute("sudo datadog-agent diagnose --verbose")
	assert.NotContains(t, diagnoseOut, "system-probe-unreachable", "diagnose should not report the issue after system-probe is running")
	t.Log("Step 2 passed: system-probe-unreachable issue resolved after starting system-probe")
}
