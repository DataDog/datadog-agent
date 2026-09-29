// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dockersocket tests container log collection through the Docker
// socket (the container tailer) against fakeintake.
package dockersocket

import (
	_ "embed"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

//go:embed testdata/docker-socket-compose.yaml
var dockerSocketCompose string

const (
	tagParityService   = "tag-parity-docker"
	tagParityContainer = "tag-parity-docker"
)

type dockerSocketSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestDockerSocketLogs runs the Docker socket log suite on an EC2 Docker host.
func TestDockerSocketLogs(t *testing.T) {
	e2e.Run(t,
		&dockerSocketSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					ec2docker.WithAgentOptions(
						dockeragentparams.WithLogs(),
						dockeragentparams.WithExtraComposeManifest("tag-parity-docker", pulumi.String(dockerSocketCompose)),
					))),
		))
}

// TestTagWireOrder pins the container tailer's ddtags order on the wire:
// parsing tags, then the tagger's container tags, then sourcecategory, then
// the configured tags from the AD label, each exactly once. The container
// tags come from the tagger and vary by environment, so only their presence
// is checked; the sourcecategory + configured tail is checked exactly. Run
// it against a main pipeline and a branch pipeline; the same assertion must
// pass on both.
func (s *dockerSocketSuite) TestTagWireOrder() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, 1*time.Minute, 5*time.Second, "Agent was not ready")

	utils.CheckLogsTagsOrderedSuffix(s.T(), s.Env().FakeIntake, tagParityService, "tag-parity-docker",
		[]string{"sourcecategory:web", "env:e2e", "team:logs"},
		[]string{"container_name:" + tagParityContainer},
	)
}
