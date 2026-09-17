// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds an entry point that runs the ORIGINAL agent-subcommands
// health suite against a local container agent managed by e2ectl — the same
// test bodies, only the provisioning differs. The environment comes from
// e2ectl-local.yml next to this file (see its header for the exact commands).
//
// This is the migration pattern: the suite and its assertions are unchanged;
// the environment is described by a config file next to the test instead of
// a Pulumi provisioner, and the entry point attaches to a live environment
// instead of provisioning one. Tests that re-provision the host (UpdateEnv
// with a Pulumi provisioner) or that need real-host capabilities (EC2
// metadata, systemd) are the attach-mode boundary — run with -run to select
// the entry points that attach cleanly.
package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// localHostSuite runs the existing health suite against the local container
// agent: TestDefaultInstallHealthy asserts `agent health` reports PASS — the
// AgentClient invokes the pinned binary directly (no sudo; the install records
// the binary path in the snapshot).
type localHealthSuite struct {
	baseHealthSuite
}

// TestLinuxHealthSuiteOnLocal runs the ORIGINAL health suite (the same suite
// TestLinuxHealthSuite runs on an EC2 VM) against a local e2ectl container
// environment.
//
// Known attach-mode boundary: TestDefaultInstallUnhealthy re-provisions the
// host with a new agent config (UpdateEnv with a Pulumi provisioner), which
// does not apply to an attached environment — skip it locally with
// `-run TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy`.
func TestLinuxHealthSuiteOnLocal(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &localHealthSuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Host](envName),
	))
}

// The status suite is NOT attached here on purpose: its assertions describe
// a full OS install — the APM agent section expects "Status: Running" (the
// trace-agent is a separate process the single core binary does not provide)
// and Autodiscovery is expected absent (the EC2 host has no container
// features; the local agent container enables them). Attaching it would be a
// failing-by-design entry point. When the binary install grows a trace-agent
// and a host-shaped (non-container) AD mode, the same plug applies.
