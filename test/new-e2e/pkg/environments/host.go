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

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// Host is an environment that contains a Host, FakeIntake and Agent configured to talk to each other.
type Host struct {
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.RemoteHostAgent
	Updater    *components.RemoteHostUpdater
}

var _ common.Initializable = (*Host)(nil)

// Init initializes the environment
func (e *Host) Init(_ common.Context) error {
	return nil
}

var _ common.Diagnosable = (*Host)(nil)

// Diagnose returns a string containing the diagnosis of the environment
func (e *Host) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.RemoteHost == nil {
		return "", fmt.Errorf("RemoteHost component is not initialized")
	}
	// add Agent diagnose
	if e.Agent != nil {
		diagnoses = append(diagnoses, "==== Agent ====")
		dstPath, err := generateAndDownloadAgentFlare(e.Agent, e.RemoteHost, outputDir)
		if err != nil {
			return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
		}
		diagnoses = append(diagnoses, fmt.Sprintf("Flare archive downloaded to %s", dstPath))
		diagnoses = append(diagnoses, "\n")
	}

	return strings.Join(diagnoses, "\n"), nil
}

func generateAndDownloadAgentFlare(agent *components.RemoteHostAgent, host *components.RemoteHost, outputDir string) (string, error) {
	if agent == nil || host == nil {
		return "", fmt.Errorf("Agent or RemoteHost component is not initialized, cannot generate flare")
	}
	// generate a local flare
	// todo skip uploading it to backend, requires further changes in agent executor
	// to redirect stdin to null, on linux adding `</dev/null`
	// on windows prepending command with `@() |`, pre-piping with an empty array
	// discard error, flare command might return error if there is no intake, but it the archive is still generated
	flareCommandOutput, err := agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send", "--local"}))

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
	flareFileInfo, err := host.Lstat(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat flare archive: %w", err)
	}
	dstPath := filepath.Join(outputDir, flareFileInfo.Name())

	err = host.EnsureFileIsReadable(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to ensure flare archive is readable: %w", err)
	}
	err = host.GetFile(flarePath, dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to download flare archive: %w", err)
	}
	return dstPath, nil
}

func (e *Host) getAgentCoverageCommands(family os.Family) (map[string]string, error) {
	switch family {
	case os.LinuxFamily:
		return map[string]string{
			"datadog-agent":  "sudo datadog-agent coverage generate",
			"trace-agent":    "sudo /opt/datadog-agent/embedded/bin/trace-agent -c /etc/datadog-agent coverage generate",
			"process-agent":  "sudo /opt/datadog-agent/embedded/bin/process-agent coverage generate",
			"security-agent": "sudo /opt/datadog-agent/embedded/bin/security-agent coverage generate",
			"system-probe":   "sudo /opt/datadog-agent/embedded/bin/system-probe coverage generate",
		}, nil
	case os.WindowsFamily:
		installPath := client.DefaultWindowsAgentInstallPath(e.RemoteHost.Host)
		return map[string]string{
			"datadog-agent":  fmt.Sprintf(`& "%s\bin\agent.exe" "coverage" "generate"`, installPath),
			"trace-agent":    fmt.Sprintf(`& "%s\bin\agent\trace-agent.exe" "coverage" "generate"`, installPath),
			"process-agent":  fmt.Sprintf(`& "%s\bin\agent\process-agent.exe" "coverage" "generate"`, installPath),
			"security-agent": fmt.Sprintf(`& "%s\bin\agent\security-agent.exe" "coverage" "generate"`, installPath),
			"system-probe":   fmt.Sprintf(`& "%s\bin\agent\system-probe.exe" "coverage" "generate"`, installPath),
		}, nil
	}
	return nil, fmt.Errorf("unsupported OS family: %v", family)
}

// Coverage runs the coverage command for each agent and downloads the coverage folders to the output directory
func (e *Host) Coverage(outputDir string) (string, error) {

	outStr := []string{}
	outStr = append(outStr, "===== Coverage =====")
	commandCoverages, err := e.getAgentCoverageCommands(e.RemoteHost.OSFamily)
	if err != nil {
		return "", err
	}
	for agent, command := range commandCoverages {
		outStr = append(outStr, fmt.Sprintf("==== %s ====", agent))
		output, err := e.RemoteHost.Execute(command)
		if err != nil {
			outStr = append(outStr, fmt.Sprintf("%s: %v", agent, err))
			continue
		}
		// find coverage folder in command output
		re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
		matches := re.FindStringSubmatch(output)
		if len(matches) < 2 {
			outStr = append(outStr, fmt.Sprintf("%s: output does not contain the path to the coverage folder, output: %s", agent, output))
			continue
		}
		outStr = append(outStr, fmt.Sprintf("Coverage folder: %s", matches[1]))
		err = e.RemoteHost.GetFolder(matches[1], filepath.Join(outputDir, filepath.Base(matches[1])))
		if err != nil {
			outStr = append(outStr, fmt.Sprintf("%s: error while getting folder:%v", agent, err))
			continue
		}
		outStr = append(outStr, fmt.Sprintf("Downloaded coverage folder: %s", matches[1]))
	}

	return strings.Join(outStr, "\n"), nil
}
