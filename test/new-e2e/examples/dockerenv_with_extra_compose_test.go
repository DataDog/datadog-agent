// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dockerSuiteWithExtraCompose struct {
	e2e.BaseSuite[environments.DockerHost]
}

//go:embed testfixtures/docker-compose.fake-process.yaml
var fakeProcessCompose string

func TestDockerWithExtraCompose(t *testing.T) {
	e2e.Run(t, &dockerSuiteWithExtraCompose{}, e2e.WithProvisioner(
		awsdocker.Provisioner(
			awsdocker.WithoutFakeIntake(),
			awsdocker.WithAgentOptions(
				dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
			),
		),
	))
}

func (v *dockerSuiteWithExtraCompose) TestListContainers() {
	t := v.T()
	containers, err := v.Env().Docker.Client.ListContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 2)
	assert.Contains(t, containers, "datadog-agent")
	assert.Contains(t, containers, "fake-process")
	v.T().Logf("Found %d containers", len(containers))
	v.T().Logf("Containers: %v", containers)
}

func (v *dockerSuiteWithExtraCompose) TestExecuteCommandStdoutStdErr() {
	t := v.T()
	stdout, stderr, err := v.Env().Docker.Client.ExecuteCommandStdoutStdErr("fake-process", "echo", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", strings.Trim(stdout, "\n"))
	assert.Empty(t, stderr)
}
