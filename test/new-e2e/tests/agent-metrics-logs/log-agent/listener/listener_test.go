// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package listener

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metrics-logs/log-agent/utils"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"

	appslogger "github.com/DataDog/test-infra-definitions/components/datadog/apps/logger"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/tcp-compose.yaml
var tcpCompose string

//go:embed testdata/udp-compose.yaml
var udpCompose string

type dockerTCPSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func (d *dockerTCPSuite) TestLogsReceived() {
	assertLogsReceived(d.T(), d.EventuallyWithT, d.Env().Agent, d.Env().Docker, d.Env().FakeIntake)
}

type dockerUDPSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func (d *dockerUDPSuite) TestLogsReceived() {
	assertLogsReceived(d.T(), d.EventuallyWithT, d.Env().Agent, d.Env().Docker, d.Env().FakeIntake)
}

func TestTCPListener(t *testing.T) {
	e2e.Run(t,
		&dockerTCPSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithAgentOptions(
					dockeragentparams.WithLogs(),
					dockeragentparams.WithExtraComposeManifest("logger", appslogger.DockerComposeManifest.Content),
					dockeragentparams.WithExtraComposeManifest("logger-tcp", pulumi.String(tcpCompose)),
				))),
	)
}

func TestUDPListener(t *testing.T) {
	e2e.Run(t,
		&dockerUDPSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithAgentOptions(
					dockeragentparams.WithLogs(),
					dockeragentparams.WithExtraComposeManifest("logger", appslogger.DockerComposeManifest.Content),
					dockeragentparams.WithExtraComposeManifest("logger-udp", pulumi.String(udpCompose)),
				))),
	)
}

func assertLogsReceived(
	t *testing.T,
	eventuallyWithT func(
		condition func(collect *assert.CollectT),
		waitFor time.Duration,
		tick time.Duration,
		msgAndArgs ...interface{}) bool,
	agent *components.DockerAgent,
	docker *components.RemoteHostDocker,
	fakeIntake *components.FakeIntake) {
	t.Helper()
	eventuallyWithT(func(c *assert.CollectT) {
		agentReady := agent.Client.IsReady()
		assert.True(c, agentReady)
	}, 1*time.Minute, 5*time.Second, "Agent was not ready")
	agentVersion := agent.Client.Version()
	t.Logf("Testing Agent Version '%v'\n", agentVersion)

	// Command to get the IP address of the logger-app container
	ipCmd := []string{
		"hostname", "-i",
	}

	ipAddress, _, err := docker.Client.ExecuteCommandStdoutStdErr("logger-app", ipCmd...)
	require.NoError(t, err)
	ipAddress = strings.TrimSpace(ipAddress)
	t.Logf("Logger-app IP address: %s", ipAddress)
	sourceHostTag := fmt.Sprintf("source_host:%s", ipAddress)
	// Command to execute inside the container
	cmd := []string{
		"/usr/local/bin/send-message.sh",
		"bob",
	}

	stdout, stderr, err := docker.Client.ExecuteCommandStdoutStdErr("logger-app", cmd...)
	require.NoError(t, err)

	t.Logf("stdout:\n\n%s\n\nstderr:\n\n%s", stdout, stderr)
	utils.CheckLogsExpected(t, fakeIntake, "test-app", "bob", []string{sourceHostTag})
}
