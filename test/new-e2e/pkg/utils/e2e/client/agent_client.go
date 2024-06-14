// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(context e2e.Context, hostOutput remote.HostOutput, waitForAgentReady bool) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily)
	params.ShouldWaitForReady = waitForAgentReady

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
		if err := waitForReadyTimeout(context.T(), host, commandRunner, agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// NewHostAgentClientWithParams creates an Agent client for host install with custom parameters
func NewHostAgentClientWithParams(context e2e.Context, hostOutput remote.HostOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily, options...)

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
		if err := waitForReadyTimeout(context.T(), host, commandRunner, agentReadyTimeout); err != nil {
			return nil, err
		}
	}
	waitForAgentsReady(context.T(), hostOutput.OSFamily, host, params)

	return commandRunner, nil
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(context e2e.Context, dockerAgentOutput agent.DockerAgentOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(dockerAgentOutput.DockerManager.Host.OSFamily, options...)
	ae := newAgentDockerExecutor(context, dockerAgentOutput)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// waitForAgentsReady waits for the given non-core agents to be ready.
// The given options configure which Agents to wait for, and how long to wait.
//
// Under the hood, this function checks the readiness of the agents by querying their status endpoints.
// The function will wait until all agents are ready, or until the timeout is reached.
// If the timeout is reached, an error is returned.
//
// As of now this is only implemented for Linux.
func waitForAgentsReady(tt *testing.T, osFamily osComp.Family, host *Host, params *agentclientparams.Params) {
	require.EventuallyWithT(tt, func(t *assert.CollectT) {
		agentReadyCmds := map[string]func(*agentclientparams.Params, osComp.Family, *Host) (string, bool, error){
			"process-agent":  processAgentCommand,
			"trace-agent":    traceAgentCommand,
			"security-agent": securityAgentCommand,
		}

		for name, getReadyCmd := range agentReadyCmds {
			cmd, ok, err := getReadyCmd(params, osFamily, host)
			if !assert.NoErrorf(t, err, "could not build ready command for %s", name) {
				continue
			}

			if !ok {
				continue
			}

			tt.Logf("Checking if %s is ready...", name)
			_, err = host.Execute(cmd)
			assert.NoErrorf(t, err, "%s did not become ready", name)
		}
	}, params.WaitForDuration, params.WaitForTick)
}

func ensureAuthToken(params *agentclientparams.Params, _ osComp.Family, host *Host) error {
	if params.AuthToken != "" {
		return nil
	}

	authToken, err := host.Execute("sudo cat " + params.AuthTokenPath)
	if err != nil {
		return fmt.Errorf("could not read auth token file: %v", err)
	}
	params.AuthToken = strings.TrimSpace(authToken)

	return nil
}

func processAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "http://localhost:%d/agent/status", params.ProcessAgentPort)
}

func traceAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "http://localhost:%d/info", params.TraceAgentPort)
}

func securityAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "https://localhost:%d/agent/status", params.SecurityAgentPort)
}

func makeStatusEndpointCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host, url string, port int) (string, bool, error) {
	if port == 0 {
		return "", false, nil
	}

	if osFamily != osComp.LinuxFamily {
		return "", true, fmt.Errorf("waiting for non-core agents is not implemented for OS family %d", osFamily)
	}

	// we want to fetch the auth token only if we actually need it
	if err := ensureAuthToken(params, osFamily, host); err != nil {
		return "", true, err
	}

	statusEndpoint := fmt.Sprintf(url, port)
	return curlCommand(statusEndpoint, params.AuthToken), true, nil
}

func curlCommand(endpoint string, authToken string) string {
	return fmt.Sprintf(
		`curl -L -s -k -H "authorization: Bearer %s" "%s"`,
		authToken,
		endpoint,
	)
}

func waitForReadyTimeout(t *testing.T, host *Host, commandRunner *agentCommandRunner, timeout time.Duration) error {
	err := commandRunner.waitForReadyTimeout(timeout)

	//if err != nil {
	// Propagate the original error if we have another error here
	localErr := generateAndDownloadFlare(t, commandRunner, host)

	if localErr != nil {
		t.Errorf("Could not generate flare: %v", localErr)
	}
	//}

	return err
}

func generateAndDownloadFlare(t *testing.T, commandRunner *agentCommandRunner, host *Host) error {
	profile := runner.GetProfile()
	outputDir, err := profile.GetOutputDir()
	//flareFound := false

	if err != nil {
		return errors.New(fmt.Sprintf("Could not get output directory: %v", err))
	}

	_, err = commandRunner.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--local"}))
	if err != nil {
		t.Errorf("Error while generating the flare: %v.", err)
		// Do not return now, the flare may be generated locally but was not uploaded because there's no fake intake
	}

	flareRegex, err := regexp.Compile(`datadog-agent-.*\.zip`)
	if err != nil {
		return errors.New(fmt.Sprintf("could not compile regex: %v", err))
	}

	tmpFolder, err := host.GetTmpFolder()
	if err != nil {
		return errors.New(fmt.Sprintf("Could not get tmp folder: %v", err))
	}

	entries, err := host.ReadDir(tmpFolder)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not read directory: %v", err))
	}

	for _, entry := range entries {
		if flareRegex.MatchString(entry.Name()) {
			t.Logf("Found flare file: %s", entry.Name())

			if host.osFamily != osComp.WindowsFamily {
				_, err = host.Execute(fmt.Sprintf("sudo chmod 744 %s/%s", tmpFolder, entry.Name()))
				if err != nil {
					return errors.New(fmt.Sprintf("Could update permission of flare file %s/%s : %v", tmpFolder, entry.Name(), err))
				}
			}

			t.Logf("Downloading flare file in: %s", outputDir)
			err = host.GetFile(fmt.Sprintf("%s/%s", tmpFolder, entry.Name()), fmt.Sprintf("%s/%s", outputDir, entry.Name()))

			if err != nil {
				return errors.New(fmt.Sprintf("Could not download flare file from %s/%s : %v", tmpFolder, entry.Name(), err))
			}

			//flareFound = true
		}
	}

	//if !flareFound {
	t.Errorf("Could not find a flare. Retrieving logs directly instead...")

	logsFolder, err := host.GetLogsFolder()
	if err != nil {
		return errors.New(fmt.Sprintf("Could not get logs folder: %v", err))
	}

	entries, err = host.ReadDir(logsFolder)

	if err != nil {
		return errors.New(fmt.Sprintf("Could not read directory: %v", err))
	}

	for _, entry := range entries {
		t.Logf("Found log file: %s. Downloading file in: %s", entry.Name(), outputDir)

		err = host.GetFile(fmt.Sprintf("%s/%s", logsFolder, entry.Name()), fmt.Sprintf("%s/%s", outputDir, entry.Name()))
		if err != nil {
			return errors.New(fmt.Sprintf("Could not download log file from %s/%s : %v", logsFolder, entry.Name(), err))
		}
	}
	//}

	return nil
}
