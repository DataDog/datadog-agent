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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// Host is an environment that contains a Host, FakeIntake and Agent configured to talk to each other.
type Host struct {
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.RemoteHostAgent
	Updater    *components.RemoteHostUpdater
}

// Ensure Host implements the HostOutputs interface for use with scenario Run functions
var _ outputs.HostOutputs = (*Host)(nil)

var _ common.Initializable = (*Host)(nil)

// Init initializes the environment
func (e *Host) Init(_ common.Context) error {
	return nil
}

// RemoteHostOutput implements outputs.HostOutputs
func (e *Host) RemoteHostOutput() *remote.HostOutput {
	if e.RemoteHost == nil {
		e.RemoteHost = &components.RemoteHost{}
	}
	return &e.RemoteHost.HostOutput
}

// FakeIntakeOutput implements outputs.HostOutputs
func (e *Host) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// AgentOutput implements outputs.HostOutputs
func (e *Host) AgentOutput() *agent.HostAgentOutput {
	if e.Agent == nil {
		e.Agent = &components.RemoteHostAgent{}
	}
	return &e.Agent.HostAgentOutput
}

// UpdaterOutput implements outputs.HostOutputs
func (e *Host) UpdaterOutput() *updater.HostUpdaterOutput {
	if e.Updater == nil {
		e.Updater = &components.RemoteHostUpdater{}
	}
	return &e.Updater.HostUpdaterOutput
}

// DisableFakeIntake implements outputs.HostOutputs
func (e *Host) DisableFakeIntake() {
	e.FakeIntake = nil
}

// DisableAgent implements outputs.HostOutputs
func (e *Host) DisableAgent() {
	e.Agent = nil
}

// DisableUpdater implements outputs.HostOutputs
func (e *Host) DisableUpdater() {
	e.Updater = nil
}

// SetAgentClientOptions implements outputs.HostOutputs
func (e *Host) SetAgentClientOptions(options ...agentclientparams.Option) {
	e.Agent.ClientOptions = options
}

var _ common.Diagnosable = (*Host)(nil)

// Diagnose returns a string containing the diagnosis of the environment
func (e *Host) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.RemoteHost == nil {
		return "", errors.New("RemoteHost component is not initialized")
	}
	// add Agent diagnose
	if e.Agent != nil {
		diagnoses = append(diagnoses, "==== Agent ====")
		dstPath, err := generateAndDownloadAgentFlare(e.Agent, e.RemoteHost, outputDir)
		if err != nil {
			return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
		}
		diagnoses = append(diagnoses, "Flare archive downloaded to "+dstPath)
		diagnoses = append(diagnoses, "\n")
	}

	return strings.Join(diagnoses, "\n"), nil
}

func generateAndDownloadAgentFlare(agent *components.RemoteHostAgent, host *components.RemoteHost, outputDir string) (string, error) {
	if agent == nil || host == nil {
		return "", errors.New("Agent or RemoteHost component is not initialized, cannot generate flare")
	}
	// generate a flare, it will fallback to local flare generation if the running agent cannot be reached
	// todo skip uploading it to backend, requires further changes in agent executor
	// to redirect stdin to null, on linux adding `</dev/null`
	// on windows prepending command with `@() |`, pre-piping with an empty array
	// discard error, flare command might return error if there is no intake, but it the archive is still generated
	flareCommandOutput, err := agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send"}))

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

func (e *Host) getAgentCoverageCommands(family os.Family) ([]CoverageTargetSpec, error) {
	switch family {
	case os.LinuxFamily:
		return []CoverageTargetSpec{{
			AgentName:       "datadog-agent",
			CoverageCommand: []string{"sudo", "datadog-agent", "coverage", "generate"},
			Required:        true,
		}, {
			AgentName:       "trace-agent",
			CoverageCommand: []string{"sudo", "/opt/datadog-agent/embedded/bin/trace-agent", "-c", "/etc/datadog-agent", "coverage", "generate"},
			Required:        false,
		}, {
			AgentName:       "process-agent",
			CoverageCommand: []string{"sudo", "/opt/datadog-agent/embedded/bin/process-agent", "coverage", "generate"},
			Required:        false,
		}, {
			AgentName:       "security-agent",
			CoverageCommand: []string{"sudo", "/opt/datadog-agent/embedded/bin/security-agent", "coverage", "generate"},
			Required:        false,
		}, {
			AgentName:       "system-probe",
			CoverageCommand: []string{"sudo", "/opt/datadog-agent/embedded/bin/system-probe", "coverage", "generate"},
			Required:        false,
		}}, nil
	case os.WindowsFamily:
		installPath := client.DefaultWindowsAgentInstallPath(e.RemoteHost.Host)
		return []CoverageTargetSpec{{
			AgentName:       "datadog-agent",
			CoverageCommand: []string{fmt.Sprintf(`& "%s\bin\agent.exe" "coverage" "generate"`, installPath)},
			Required:        true,
		}, {
			AgentName:       "trace-agent",
			CoverageCommand: []string{fmt.Sprintf(`& "%s\bin\agent\trace-agent.exe" "coverage" "generate"`, installPath)},
			Required:        false,
		}, {
			AgentName:       "process-agent",
			CoverageCommand: []string{fmt.Sprintf(`& "%s\bin\agent\process-agent.exe" "coverage" "generate"`, installPath)},
			Required:        false,
		}, {
			AgentName:       "security-agent",
			CoverageCommand: []string{fmt.Sprintf(`& "%s\bin\agent\security-agent.exe" "coverage" "generate"`, installPath)},
			Required:        false,
		}, {
			AgentName:       "system-probe",
			CoverageCommand: []string{fmt.Sprintf(`& "%s\bin\agent\system-probe.exe" "coverage" "generate"`, installPath)},
			Required:        false,
		}}, nil
	}
	return nil, fmt.Errorf("unsupported OS family: %v", family)
}

// Coverage runs the coverage command for each agent and downloads the coverage folders to the output directory
func (e *Host) Coverage(outputDir string) (string, error) {

	if e.RemoteHost == nil {
		return "RemoteHost component is not initialized, skipping coverage", nil
	}
	if e.Agent == nil {
		return "Agent component is not initialized, skipping coverage", nil
	}

	outStr := []string{}
	outStr = append(outStr, "===== Coverage =====")
	commandCoverages, err := e.getAgentCoverageCommands(e.RemoteHost.OSFamily)
	if err != nil {
		return "", err
	}
	errs := []error{}
	for _, target := range commandCoverages {
		outStr = append(outStr, fmt.Sprintf("==== %s ====", target.AgentName))
		output, err := e.RemoteHost.Execute(strings.Join(target.CoverageCommand, " "))
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}
		// find coverage folder in command output
		re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
		matches := re.FindStringSubmatch(output)
		if len(matches) < 2 {
			outStr, errs = updateErrorOutput(target, outStr, errs, "output does not contain the path to the coverage folder, output: "+output)
			continue
		}
		err = e.RemoteHost.GetFolder(matches[1], filepath.Join(outputDir, filepath.Base(matches[1])))
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}
		outStr = append(outStr, "Downloaded coverage folder: "+matches[1])
	}

	if len(errs) > 0 {
		return strings.Join(outStr, "\n"), errors.Join(errs...)
	}
	return strings.Join(outStr, "\n"), nil
}
