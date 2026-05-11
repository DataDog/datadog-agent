// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains shared helpers for agent-metric-pipelines e2e tests.
//
// The ADP helpers in this file enable the Agent Data Plane (ADP) for tests so
// that DogStatsD traffic is served by ADP instead of the Core Agent's
// DogStatsD pipeline. This is used to surface functional compatibility gaps
// between ADP and Core Agent. See DADP-72.
package common

import (
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// WithADPEnabled returns an agentparams option that enables the Agent Data
// Plane and routes DogStatsD traffic through it.
//
// It appends to ExtraAgentConfig (instead of overwriting AgentConfig) so it
// composes with existing WithAgentConfig calls in the same provisioner.
func WithADPEnabled() func(*agentparams.Params) error {
	return func(p *agentparams.Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig,
			pulumi.String("data_plane.enabled: true"),
			pulumi.String("data_plane.dogstatsd.enabled: true"),
		)
		return nil
	}
}

// AssertADPRunning fails the test if UDP port 8125 is not bound by the
// agent-data-plane process on the remote host. Use this at the start of any
// suite that relies on ADP serving DogStatsD traffic — without it, a
// misconfigured agent will silently fall back to the Core Agent DogStatsD
// pipeline and produce a false green.
//
// Linux-only (matches the existing *_nix_test.go suites).
func AssertADPRunning(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	ok := assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// ss -lnup lists listening UDP sockets with process info. The owning
		// process appears as users:(("agent-data-plane",pid=...,fd=...)).
		out, err := host.Execute("sudo ss -lnup 'sport = :8125'")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, out, "agent-data-plane",
			"UDP/8125 should be bound by agent-data-plane; got: %s", out)
	}, 2*time.Minute, 5*time.Second,
		"timed out waiting for agent-data-plane to bind UDP/8125")

	if !ok {
		// Surface the agent-data-plane systemd log so CI failures don't require
		// SSHing into a torn-down host to figure out why ADP didn't start.
		// Mirrors the journalctl-on-failure pattern in agent-platform's
		// CheckADPEnabled (test/new-e2e/tests/agent-platform/common/agent_behaviour.go).
		journal, _ := host.Execute("sudo journalctl -u agent-data-plane --no-pager -n 200")
		require.FailNowf(t, "agent-data-plane did not bind UDP/8125",
			"agent-data-plane journalctl tail:\n%s", journal)
	}
}
