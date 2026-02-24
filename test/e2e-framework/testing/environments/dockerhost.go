// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// DockerHost is an environment that contains a Docker VM, FakeIntake and Agent configured to talk to each other.
type DockerHost struct {
	// Components
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent
	Docker     *components.RemoteHostDocker
}

// Ensure DockerHost implements the DockerHostOutputs interface
var _ outputs.DockerHostOutputs = (*DockerHost)(nil)

var _ common.Initializable = &DockerHost{}

// Init initializes the environment
func (e *DockerHost) Init(_ common.Context) error {
	return nil
}

// RemoteHostOutput implements outputs.DockerHostOutputs
func (e *DockerHost) RemoteHostOutput() *remote.HostOutput {
	if e.RemoteHost == nil {
		e.RemoteHost = &components.RemoteHost{}
	}
	return &e.RemoteHost.HostOutput
}

// FakeIntakeOutput implements outputs.DockerHostOutputs
func (e *DockerHost) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// DockerAgentOutput implements outputs.DockerHostOutputs
func (e *DockerHost) DockerAgentOutput() *agent.DockerAgentOutput {
	if e.Agent == nil {
		e.Agent = &components.DockerAgent{}
	}
	return &e.Agent.DockerAgentOutput
}

// DockerOutput implements outputs.DockerHostOutputs
func (e *DockerHost) DockerOutput() *docker.ManagerOutput {
	if e.Docker == nil {
		e.Docker = &components.RemoteHostDocker{}
	}
	return &e.Docker.ManagerOutput
}

// DisableFakeIntake implements outputs.DockerHostOutputs
func (e *DockerHost) DisableFakeIntake() {
	e.FakeIntake = nil
}

// DisableAgent implements outputs.DockerHostOutputs
func (e *DockerHost) DisableAgent() {
	e.Agent = nil
}

var _ common.Diagnosable = (*DockerHost)(nil)

// Diagnose returns a string containing the diagnosis of the environment
func (e *DockerHost) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.Docker == nil {
		return "", errors.New("Docker component is not initialized")
	}
	// add Agent diagnose
	if e.Agent == nil {
		return "", errors.New("Agent component is not initialized")
	}

	diagnoses = append(diagnoses, "==== Agent ====")
	dstPath, err := e.generateAndDownloadAgentFlare(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
	}
	diagnoses = append(diagnoses, "Flare archive downloaded to "+dstPath)
	diagnoses = append(diagnoses, "\n")

	return strings.Join(diagnoses, "\n"), nil
}

// Coverage generates a coverage report for the Docker agent
func (e *DockerHost) Coverage(outputDir string) (string, error) {
	if e.Docker == nil {
		return "Docker component is not initialized, skipping coverage", nil
	}
	if e.Agent == nil {
		return "Agent component is not initialized, skipping coverage", nil
	}

	outStr := []string{"===== Coverage ====="}
	outStr = append(outStr, "==== Docker Agent ====")

	result, err := e.generateAndDownloadCoverageForContainer(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to generate and download coverage for container: %w", err)
	}
	outStr = append(outStr, result)

	return strings.Join(outStr, "\n"), err
}

// getAgentCoverageCommands returns the coverage commands for each agent component
func (e *DockerHost) getAgentCoverageCommands() []CoverageTargetSpec {
	return []CoverageTargetSpec{
		{
			AgentName:       "agent",
			CoverageCommand: []string{"agent", "coverage", "generate"},
			Required:        true,
		},
		{
			AgentName:       "trace-agent",
			CoverageCommand: []string{"trace-agent", "coverage", "generate", "-c", "/etc/datadog-agent/datadog.yaml"},
			Required:        false,
		},
		{
			AgentName:       "process-agent",
			CoverageCommand: []string{"process-agent", "coverage", "generate"},
			Required:        false,
		},
		{
			AgentName:       "security-agent",
			CoverageCommand: []string{"security-agent", "coverage", "generate"},
			Required:        false,
		},
		{
			AgentName:       "system-probe",
			CoverageCommand: []string{"system-probe", "coverage", "generate"},
			Required:        false,
		},
	}
}

func (e *DockerHost) generateAndDownloadCoverageForContainer(outputDir string) (string, error) {
	commandCoverages := e.getAgentCoverageCommands()
	outStr := []string{}
	errs := []error{}
	for _, target := range commandCoverages {
		outStr = append(outStr, fmt.Sprintf("Component %s:\n", target.AgentName))

		// Execute coverage command in the Docker container
		stdout, err := e.Docker.Client.ExecuteCommandWithErr(e.Agent.ContainerName, target.CoverageCommand...)
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}

		// find coverage folder in command output
		re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
		matches := re.FindStringSubmatch(stdout)
		if len(matches) < 2 {
			outStr, errs = updateErrorOutput(target, outStr, errs, "output does not contain the path to the coverage folder, output: "+stdout)
			continue
		}

		coveragePath := matches[1]

		// Create local destination directory
		localCoverageDir := filepath.Join(outputDir, "coverage")

		// Download the coverage folder from the Docker container
		err = e.Docker.Client.DownloadFile(e.Agent.ContainerName, coveragePath, localCoverageDir)
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}

		outStr = append(outStr, fmt.Sprintf("Downloaded coverage folder: %s to %s", coveragePath, localCoverageDir))
	}

	if len(errs) > 0 {
		return strings.Join(outStr, "\n"), errors.Join(errs...)
	}
	return strings.Join(outStr, "\n"), nil
}

func (e *DockerHost) generateAndDownloadAgentFlare(outputDir string) (string, error) {
	if e.Agent == nil || e.Docker == nil {
		return "", errors.New("Agent or Docker component is not initialized, cannot generate flare")
	}
	// generate a flare, it will fallback to local flare generation if the running agent cannot be reached
	// discard error, flare command might return error if there is no intake, but the archive is still generated
	flareCommandOutput, err := e.Agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send"}))

	lines := []string{flareCommandOutput}
	if err != nil {
		lines = append(lines, err.Error())
	}
	// on error, the flare output is in the error message
	flareCommandOutput = strings.Join(lines, "\n")

	// find <path to flare>.zip in flare command output
	// (?m) is a flag that allows ^ and $ to match the beginning and end of each line
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	matches := re.FindStringSubmatch(flareCommandOutput)
	if len(matches) < 2 {
		return "", fmt.Errorf("output does not contain the path to the flare archive, output: %s", flareCommandOutput)
	}
	flarePath := matches[1]

	// Get the filename from the flare path for the local destination
	flareFilename := filepath.Base(flarePath)
	dstPath := filepath.Join(outputDir, flareFilename)

	// Download the flare file from the Docker container to the local filesystem
	err = e.Docker.Client.DownloadFile(e.Agent.ContainerName, flarePath, dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to download flare archive from container: %w", err)
	}

	return dstPath, nil
}
