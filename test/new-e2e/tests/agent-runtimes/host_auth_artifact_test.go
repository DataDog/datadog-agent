// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the cloud-backed (ec2-host base) attach entry point for the
// ORIGINAL auth-artifact IPC security suite (authArtifactLinux, the same
// suite TestAuthArtifactIPCSecurityLinuxSuite runs on a Pulumi-provisioned
// EC2 VM). The environment contract is e2ectl-host-auth-artifact.yml next to
// this file.
package agentruntimes

import (
	"testing"
)

// TestAuthArtifactIPCSecurityLinuxSuiteOnHost is the migration contract for
// the auth-artifact suite on an e2ectl ec2-host environment: it will run the
// ORIGINAL authArtifactLinux body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today for two reasons. First, install inputs are inexpressible:
// the suite provisions agentparams.WithSecurityAgentConfig
// (auth_artifact/fixtures/security-agent.yaml ->
// /etc/datadog-agent/security-agent.yaml) and
// agentparams.WithSystemProbeConfig (network_config -> system-probe.yaml),
// plus the agentclientparams.WithSkipWaitForAgentReady client option — the
// agent.script section covers datadog.yaml extras and conf.d integrations
// only. Second, the test body drives the systemd lifecycle of all four agent
// processes (svcManager.Start/Stop on the datadog-agent unit), which needs a
// real systemd host — that part is what the ec2-host base is for. Remove
// this skip when the script installer grows security-agent/system-probe
// config inputs (the systemd host is already the base).
func TestAuthArtifactIPCSecurityLinuxSuiteOnHost(t *testing.T) {
	t.Skip("pending: security-agent.yaml/system-probe.yaml config inputs in the agent.script installer; see the boundary comment on this entry point")
}
