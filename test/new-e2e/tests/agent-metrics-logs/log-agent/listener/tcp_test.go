// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package listener

import (
	_ "embed"
	"testing"
	"time"

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

type dockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestTCPListener(t *testing.T) {
	e2e.Run(t,
		&dockerSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithAgentOptions(
					dockeragentparams.WithLogs(),
					dockeragentparams.WithExtraComposeManifest("logger", appslogger.DockerComposeManifest.Content),
					dockeragentparams.WithExtraComposeManifest("logger-tcp", pulumi.String(tcpCompose)),
				))),
	)
}

func (d *dockerSuite) TestLogsReceived() {
	d.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := d.Env().Agent.Client.IsReady()
		assert.True(c, agentReady)
	}, 1*time.Minute, 5*time.Second, "Agent was not ready")
	agentVersion := d.Env().Agent.Client.Version()
	d.T().Logf("Testing Agent Version '%v'\n", agentVersion)
	statusOutput := d.Env().Agent.Client.Status().Content
	d.T().Logf("Agent status:\n %v", statusOutput)

	// Command to execute inside the container
	cmd := []string{
		"/usr/local/bin/send-message.sh",
		"bob",
	}

	stdout, stderr, err := d.Env().Docker.Client.ExecuteCommandStdoutStdErr("logger-app", cmd...)
	require.NoError(d.T(), err)
	assert.Empty(d.T(), stderr)
	d.T().Logf("stdout:\n\n%s\n\nstderr:\n\n%s", stdout, stderr)
	utils.CheckLogsExpected(d.T(), d.Env().FakeIntake, "test-app", "bob", []string{})
}
