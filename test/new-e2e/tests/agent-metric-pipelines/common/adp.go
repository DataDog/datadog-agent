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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
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

// WithADPEnabledDocker is the equivalent of WithADPEnabled for the
// containerized Agent. dockeragentparams doesn't expose AgentConfig directly;
// ADP is enabled via environment variables on the Agent service instead.
//
// JMXFetch's default reporter is DogStatsD — it scrapes JMX MBeans from the
// target JVM and refeeds the metrics into the local DSD port (8125), which
// ADP intercepts when enabled. So jmxfetch tests under ADP exercise ADP's
// real data path, not just a smoke test.
func WithADPEnabledDocker() func(*dockeragentparams.Params) error {
	return func(p *dockeragentparams.Params) error {
		p.AgentServiceEnvironment["DD_DATA_PLANE_ENABLED"] = pulumi.String("true")
		p.AgentServiceEnvironment["DD_DATA_PLANE_DOGSTATSD_ENABLED"] = pulumi.String("true")
		return nil
	}
}

// AssertADPRunningDocker is the Docker equivalent of AssertADPRunning. It
// shells out from the test host to docker and checks that UDP port 8125 is
// bound by the agent-data-plane process inside the Agent container.
//
// The Agent container name comes from the e2e DockerAgent component. Checks
// socket ownership against the truncated comm form ("agent-data-plan", 15
// chars — see AssertADPRunning for the kernel-quirk context).
func AssertADPRunningDocker(t *testing.T, host *components.RemoteHost, agentContainerName string) {
	t.Helper()
	ok := assert.EventuallyWithT(t, func(c *assert.CollectT) {
		running, err := host.Execute(fmt.Sprintf("sudo docker inspect --format '{{.State.Running}}' %q", agentContainerName))
		if !assert.NoError(c, err, "agent container %q not found", agentContainerName) {
			return
		}
		if !assert.Equal(c, "true", strings.TrimSpace(running), "agent container %q is not running", agentContainerName) {
			return
		}

		// ss shows the process name as "agent-data-plan" for the same
		// TASK_COMM_LEN reason as AssertADPRunning.
		out, err := host.Execute(fmt.Sprintf("sudo docker exec %q ss -lnup 'sport = :8125'", agentContainerName))
		if !assert.NoError(c, err, "failed checking UDP/8125 ownership in agent container") {
			return
		}
		assert.Contains(c, out, "agent-data-plan",
			"UDP/8125 should be bound by agent-data-plane inside the agent container (shown as truncated 'agent-data-plan' by ss); got: %s", out)
	}, 2*time.Minute, 5*time.Second,
		"timed out waiting for agent-data-plane to bind UDP/8125 in the agent container")

	if !ok {
		// Best-effort: pull container logs for the agent to help diagnose what happened.
		logs, _ := host.Execute(fmt.Sprintf("sudo docker logs --tail 200 %q 2>&1", agentContainerName))
		socketInfo, _ := host.Execute(fmt.Sprintf("sudo docker exec %q ss -lnup 'sport = :8125' 2>&1", agentContainerName))
		containers, _ := host.Execute("sudo docker ps -a --format '{{.Names}} {{.Image}} {{.Status}}'")
		require.FailNowf(t, "agent-data-plane did not bind UDP/8125 in agent container",
			"agent container %q UDP/8125 sockets:\n%s\nlogs tail:\n%s\ncontainers:\n%s", agentContainerName, socketInfo, logs, containers)
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
		// process appears as users:(("agent-data-plan",pid=...,fd=...)).
		//
		// Note: the process name shows as "agent-data-plan" (15 chars), not
		// "agent-data-plane" (16 chars). This is a Linux kernel limitation —
		// /proc/<pid>/comm caps process names at TASK_COMM_LEN = 16 (15 chars
		// + null), so the full 16-char binary name is truncated by one
		// character. Match on the truncated form.
		out, err := host.Execute("sudo ss -lnup 'sport = :8125'")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, out, "agent-data-plan",
			"UDP/8125 should be bound by agent-data-plane (shown as truncated 'agent-data-plan' by ss); got: %s", out)
	}, 2*time.Minute, 5*time.Second,
		"timed out waiting for agent-data-plane to bind UDP/8125")

	if !ok {
		// Surface the agent-data-plane systemd log so CI failures don't require
		// SSHing into a torn-down host to figure out why ADP didn't start.
		// The unit is named "datadog-agent-data-plane" on installed packages
		// but "agent-data-plane" in some dev builds — query both.
		journal, _ := host.Execute("sudo journalctl -u agent-data-plane -u datadog-agent-data-plane --no-pager -n 200")
		require.FailNowf(t, "agent-data-plane did not bind UDP/8125",
			"agent-data-plane journalctl tail:\n%s", journal)
	}
}
