// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the cloud-backed (ec2-host base) attach entry point for the
// ORIGINAL IPC security suite (ipcSecurityLinuxSuite, the same suite
// TestIPCSecurityLinuxSuite runs on a Pulumi-provisioned EC2 VM). The
// environment contract is e2ectl-host-ipc-security.yml next to this file.
package agentruntimes

import (
	"testing"
)

// TestIPCSecurityLinuxSuiteOnHost is the migration contract for the IPC
// security suite on an e2ectl ec2-host environment: it will run the ORIGINAL
// ipcSecurityLinuxSuite body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today because the suite's only test starts by re-provisioning the
// environment: TestServersideIPCCertUsage calls s.UpdateEnv(awshost.Provisioner(...))
// with the templated IPC cert/ports datadog.yaml, a security-agent.yaml, a
// system-probe config, and per-agent client port rewiring
// (agentclientparams.WithTraceAgentOnPort/... — the mutator must rewrite the
// client wiring in the snapshot, not just the YAML; see the configrefresh
// case in the gap analysis). Three seams are pending: the config mutator
// (gap-analysis Step 1), agentclientparams rewiring through it, and
// security-agent/system-probe config inputs in the script installer. Remove
// this skip when they exist.
func TestIPCSecurityLinuxSuiteOnHost(t *testing.T) {
	t.Skip("pending: per-test UpdateEnv config mutation with agentclientparams rewiring (gap-analysis Step 1) plus security-agent/system-probe config inputs; see the boundary comment on this entry point")
}
