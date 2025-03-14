// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// DockerHost is an environment that contains a Docker VM, FakeIntake and Agent configured to talk to each other.
type DockerHost struct {
	// Components
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent
	Docker     *components.RemoteHostDocker
}

var _ common.Initializable = &DockerHost{}

// Init initializes the environment
func (e *DockerHost) Init(_ common.Context) error {
	return nil
}

// SetupCoverage ask the agent to create a temporary folder for coverage files and returns the path
func (e *DockerHost) SetupCoverage() (string, error) {
	if e.Agent == nil || e.Docker == nil {
		return "", fmt.Errorf("Agent component is not initialized, cannot create coverage folder")
	}
	r, err := e.Agent.Client.CoverageWithError(agentclient.WithArgs([]string{"generate"}))
	if err != nil {
		return "", fmt.Errorf("failed to generate coverage: %w", err)
	}
	// find coverage folder in command output
	re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
	matches := re.FindStringSubmatch(r)
	if len(matches) < 2 {
		return "", fmt.Errorf("output does not contain the path to the coverage folder, output: %s", r)
	}
	coveragePath := matches[1]
	return coveragePath, nil
}

// Coverage generates coverage files and downloads them to the given output directory
func (e *DockerHost) Coverage(remoteCoverageDir, outputDir string) error {
	if e.Agent == nil || e.Docker == nil {
		return fmt.Errorf("Agent component is not initialized, cannot generate coverage")
	}
	if _, err := e.Agent.Client.CoverageWithError(agentclient.WithArgs([]string{"generate"})); err != nil {
		return fmt.Errorf("failed to generate coverage: %w", err)
	}

	//TODO: Implement a method on docker client insead
	_, err := e.RemoteHost.Execute(fmt.Sprintf("docker cp %s:%s %s", e.Agent.ContainerName, remoteCoverageDir, remoteCoverageDir))
	if err != nil {
		return fmt.Errorf("failed to get coverage: %w", err)
	}
	err = e.RemoteHost.GetFolder(remoteCoverageDir, outputDir)
	if err != nil {
		return fmt.Errorf("failed to get coverage: %w", err)
	}

	return nil
}
