// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

// This example shows how to use fakeintake's Remote Config backend to change
// the agent's log level at runtime.
//
// Remote Config is wired automatically: every awshost.Provisioner() run
// starts fakeintake with a fixed TUF signing key and configures the agent to
// point at fakeintake's RC endpoint — no extra provisioner options needed.
//
// Flow:
//  1. Agent starts at the default (info) log level — no DEBUG lines in the log.
//  2. Two AGENT_CONFIG payloads are pushed via the fakeintake RC API:
//     - a named layer that sets log_level to "debug"
//     - a configuration_order that activates the layer
//  3. The agent polls fakeintake (every 5 s), receives the signed config, and
//     starts writing DEBUG lines to its log file.

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type rcLogLevelExampleSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestRCLogLevelExampleSuite is the entry point for the Remote Config log-level example.
// Run locally with:
//
//	dda inv new-e2e-tests.run --targets=./examples/... -run TestRCLogLevelExampleSuite
func TestRCLogLevelExampleSuite(t *testing.T) {
	// RC is wired automatically — no special provisioner option is required.
	e2e.Run(t, &rcLogLevelExampleSuite{},
		e2e.WithProvisioner(awshost.Provisioner()),
	)
}

// TestLogLevelViaRC verifies that the agent honours an AGENT_CONFIG payload
// delivered through Remote Config:
//
//  1. No DEBUG lines exist at startup (default log_level is info).
//  2. After pushing an AGENT_CONFIG layer + configuration_order, DEBUG lines appear.
func (s *rcLogLevelExampleSuite) TestLogLevelViaRC() {
	rh := s.Env().RemoteHost
	fi := s.Env().FakeIntake.Client()

	// Step 1 — confirm the agent is up and not yet producing DEBUG logs.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "agent did not become ready")

	agentLog, err := rh.ReadFilePrivileged("/var/log/datadog/agent.log")
	require.NoError(s.T(), err)
	require.False(s.T(), strings.Contains(string(agentLog), "| DEBUG |"),
		"expected no DEBUG logs at default log level")

	// Step 2 — push an AGENT_CONFIG layer that sets log_level to "debug".
	err = fi.RCAddConfig("", "AGENT_CONFIG", "layer1", "log_level_debug",
		[]byte(`{"name":"layer1","config":{"log_level":"debug"}}`))
	require.NoError(s.T(), err)

	// Step 3 — push a configuration_order to activate the layer.
	// MergeRCAgentConfig requires a non-empty Order or InternalOrder to apply.
	err = fi.RCAddConfig("", "AGENT_CONFIG", "configuration_order", "order",
		[]byte(`{"order":["layer1"],"internal_order":[]}`))
	require.NoError(s.T(), err)

	// Step 4 — wait for DEBUG lines to appear.
	// The agent polls RC every 5 s (set by remote_configuration.refresh_interval).
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := rh.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err)
		assert.True(c, strings.Contains(string(logs), "| DEBUG |"),
			"expected DEBUG logs after RC log level change")
	}, 3*time.Minute, 10*time.Second)
}
