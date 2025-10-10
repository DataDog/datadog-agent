// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

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

var _ common.Diagnosable = (*DockerHost)(nil)

// Diagnose returns a string containing the diagnosis of the environment
func (e *DockerHost) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.Docker == nil {
		return "", fmt.Errorf("Docker component is not initialized")
	}
	// add Agent diagnose
	if e.Agent == nil {
		return "", fmt.Errorf("Agent component is not initialized")
	}

	diagnoses = append(diagnoses, "==== Agent ====")
	dstPath, err := e.generateAndDownloadAgentFlare(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
	}
	diagnoses = append(diagnoses, fmt.Sprintf("Flare archive downloaded to %s", dstPath))
	diagnoses = append(diagnoses, "\n")

	return strings.Join(diagnoses, "\n"), nil
}

// Coverage generates a coverage report for the Docker agent
func (e *DockerHost) Coverage(outputDir string) (string, error) {
	if e.Docker == nil {
		return "", fmt.Errorf("Docker component is not initialized")
	}
	if e.Agent == nil {
		return "", fmt.Errorf("Agent component is not initialized")
	}

	outStr := []string{"===== Coverage ====="}
	outStr = append(outStr, "==== Docker Agent ====")

	result := e.generateAndDownloadCoverageForContainer(outputDir)
	outStr = append(outStr, result)

	return strings.Join(outStr, "\n"), nil
}

// getAgentCoverageCommands returns the coverage commands for each agent component
func (e *DockerHost) getAgentCoverageCommands() map[string][]string {
	return map[string][]string{
		"agent":          {"agent", "coverage", "generate"},
		"trace-agent":    {"trace-agent", "coverage", "generate", "-c", "/etc/datadog-agent/datadog.yaml"},
		"process-agent":  {"process-agent", "coverage", "generate"},
		"security-agent": {"security-agent", "coverage", "generate"},
		"system-probe":   {"system-probe", "coverage", "generate"},
	}
}

func (e *DockerHost) generateAndDownloadCoverageForContainer(outputDir string) string {
	commandCoverages := e.getAgentCoverageCommands()
	outStr := []string{}

	for component, command := range commandCoverages {
		outStr = append(outStr, fmt.Sprintf("Component %s:\n", component))

		// Execute coverage command in the Docker container
		stdout, err := e.Docker.Client.ExecuteCommandWithErr(e.Agent.ContainerName, command...)
		if err != nil {
			outStr = append(outStr, fmt.Sprintf("Error: %v\n", err))
			continue
		}

		// find coverage folder in command output
		re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
		matches := re.FindStringSubmatch(stdout)
		if len(matches) < 2 {
			outStr = append(outStr, fmt.Sprintf("Error: output does not contain the path to the coverage folder, output: %s", stdout))
			continue
		}

		coveragePath := matches[1]

		// Create local destination directory
		localCoverageDir := filepath.Join(outputDir, "coverage")

		// Download the coverage folder from the Docker container
		err = e.Docker.Client.DownloadFile(e.Agent.ContainerName, coveragePath, localCoverageDir)
		if err != nil {
			outStr = append(outStr, fmt.Sprintf("Error: error while downloading coverage folder: %v\n", err))
			continue
		}

		outStr = append(outStr, fmt.Sprintf("Downloaded coverage folder: %s to %s", coveragePath, localCoverageDir))
	}

	return strings.Join(outStr, "\n")
}

func (e *DockerHost) generateAndDownloadAgentFlare(outputDir string) (string, error) {
	if e.Agent == nil || e.Docker == nil {
		return "", fmt.Errorf("Agent or Docker component is not initialized, cannot generate flare")
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
