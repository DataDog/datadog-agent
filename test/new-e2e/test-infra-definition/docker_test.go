// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/dockerparams"
)

type dockerSuite struct {
	e2e.Suite[e2e.DockerEnv]
}

func TestDocker(t *testing.T) {
	e2e.Run(t, &dockerSuite{}, e2e.DockerStackDef(dockerparams.WithAgent()))
}

func (v *dockerSuite) TestExecuteCommand() {
	docker := v.Env().Docker
	output := docker.ExecuteCommand(docker.GetAgentContainerName(), "agent", "version")
	regexpVersion := regexp.MustCompile(`.*Agent .* - Commit: .* - Serialization version: .* - Go version: .*`)

	v.Require().Truef(regexpVersion.MatchString(output), fmt.Sprintf("%v doesn't match %v", output, regexpVersion))

	// args is used to test client.WithArgs. The values of the arguments are not relevant.
	args := client.WithArgs([]string{"-n", "-c", "."})
	version := docker.GetAgent().Version(args)
	v.Require().Truef(regexpVersion.MatchString(version), fmt.Sprintf("%v doesn't match %v", version, regexpVersion))
}
