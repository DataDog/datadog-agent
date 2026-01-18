// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	componentos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	agentclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	boundport "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	pkgmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/pkg-manager"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"

	"testing"
)

type tHelper interface {
	Helper()
}

// GetServiceManager returns the service manager for the host
func GetServiceManager(host *components.RemoteHost) svcmanager.ServiceManager {
	if _, err := host.Execute("command -v systemctl"); err == nil {
		return svcmanager.NewSystemctl(host)
	}

	if _, err := host.Execute("command -v /sbin/initctl"); err == nil {
		return svcmanager.NewUpstart(host)
	}

	if _, err := host.Execute("command -v service"); err == nil {
		return svcmanager.NewService(host)
	}
	return nil
}

func getPackageManager(host *components.RemoteHost) pkgmanager.PackageManager {
	if _, err := host.Execute("command -v apt"); err == nil {
		return pkgmanager.NewApt(host)
	}

	if _, err := host.Execute("command -v yum"); err == nil {
		return pkgmanager.NewYum(host)
	}

	if _, err := host.Execute("command -v zypper"); err == nil {
		return pkgmanager.NewZypper(host)
	}

	return nil
}

// TestClient contain the Agent Env and SvcManager and PkgManager for tests
type TestClient struct {
	Host        *components.RemoteHost
	AgentClient agentclient.Agent
	Helper      helpers.Helper
	FileManager filemanager.FileManager
	SvcManager  svcmanager.ServiceManager
	PkgManager  pkgmanager.PackageManager
}

// NewTestClient create a an ExtendedClient from VMClient and AgentCommandRunner, includes svcManager and pkgManager to write agent-platform tests
func NewTestClient(host *components.RemoteHost, agentClient agentclient.Agent, fileManager filemanager.FileManager, helper helpers.Helper) *TestClient {
	svcManager := GetServiceManager(host)
	pkgManager := getPackageManager(host)
	return &TestClient{
		Host:        host,
		AgentClient: agentClient,
		Helper:      helper,
		FileManager: fileManager,
		SvcManager:  svcManager,
		PkgManager:  pkgManager,
	}
}

// SetConfig set config given a key and a path to a yaml config file, supports arbitrarily-deep nested keys
func (c *TestClient) SetConfig(confPath string, key string, value string) error {
	confYaml := map[string]any{}
	conf, err := c.FileManager.ReadFile(confPath)
	if err != nil {
		fmt.Printf("config file: %s not found, it will be created\n", confPath)
	}
	if err := yaml.Unmarshal(conf, &confYaml); err != nil {
		return err
	}

	// Normalize the map and then recurse through it, creating the necessary structure before
	// inserting the value at the intended depth.
	confYaml = normalizeMap(confYaml).(map[string]any)

	keyList := strings.Split(key, ".")

	current := confYaml
	for i := 0; i < len(keyList)-1; i++ {
		k := keyList[i]
		if current[k] == nil {
			current[k] = map[string]any{}
			current = current[k].(map[string]any)
		} else {
			switch v := current[k].(type) {
			case map[string]any:
				current = v
			default:
				current[k] = map[string]any{}
				current = current[k].(map[string]any)
			}
		}
	}

	current[keyList[len(keyList)-1]] = value

	confUpdated, err := yaml.Marshal(confYaml)
	if err != nil {
		return err
	}
	_, err = c.FileManager.WriteFile(confPath, confUpdated)
	return err
}

// normalizeMap converts map[interface{}]any to map[string]any recursively
func normalizeMap(input any) any {
	switch v := input.(type) {
	case map[interface{}]any:
		result := make(map[string]any)
		for key, value := range v {
			if strKey, ok := key.(string); ok {
				result[strKey] = normalizeMap(value)
			}
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for key, value := range v {
			result[key] = normalizeMap(value)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = normalizeMap(item)
		}
		return result
	default:
		return v
	}
}

// GetJSONStatus returns the status of the Agent in JSON format
func (c *TestClient) GetJSONStatus() (map[string]any, error) {
	statusJSON := map[string]any{}
	ok := false
	var statusString string

	for try := 0; try < 60 && !ok; try++ {
		status, err := c.AgentClient.StatusWithError(agentclient.WithArgs([]string{"-j"}))
		if err == nil {
			ok = true
			statusString = status.Content
		}
		time.Sleep(1 * time.Second)
	}

	err := json.Unmarshal([]byte(statusString), &statusJSON)
	if err != nil {
		fmt.Println("Failed to unmarshal status content: ", statusString)
		if c.Host.OSFamily == componentos.LinuxFamily {
			// TEMPORARY DEBUG: on error print logs from journalctx
			output, err := c.Host.Execute("journalctl -u datadog-agent")
			if err != nil {
				fmt.Println("Failed to get logs from journalctl, ignoring... ")
			} else {
				fmt.Println("Logs from journalctl: ", output)
			}
		}

		return nil, err
	}
	return statusJSON, nil
}

// GetPythonVersion returns python version from the Agent status
func (c *TestClient) GetPythonVersion() (string, error) {
	statusJSON, err := c.GetJSONStatus()
	if err != nil {
		return "", err
	}
	pythonVersion := statusJSON["python_version"].(string)

	return pythonVersion, nil
}

// GetAgentVersion returns agent version from the Agent status
func (c *TestClient) GetAgentVersion() (string, error) {
	statusJSON, err := c.GetJSONStatus()
	if err != nil {
		return "", err
	}
	agentVersion := statusJSON["version"].(string)

	return agentVersion, nil
}

// ExecuteWithRetry execute the command with retry
func (c *TestClient) ExecuteWithRetry(cmd string) (string, error) {
	return execWithRetry(func(cmd string) (string, error) { return c.Host.Execute(cmd) }, cmd)
}

// NewWindowsTestClient create a TestClient for Windows VM
func NewWindowsTestClient(context common.Context, host *components.RemoteHost) *TestClient {
	fileManager := filemanager.NewRemoteHost(host)
	t := context.T()

	agentClient, err := client.NewHostAgentClient(context, host.HostOutput, false)
	require.NoError(t, err)

	helper := helpers.NewWindowsHelper()
	client := NewTestClient(host, agentClient, fileManager, helper)
	client.SvcManager = svcmanager.NewWindows(host)

	return client
}

// RunningAgentProcesses returns the list of running agent processes
func RunningAgentProcesses(client *TestClient) ([]string, error) {
	agentProcesses := client.Helper.AgentProcesses()
	runningAgentProcesses := []string{}
	for _, process := range agentProcesses {
		if AgentProcessIsRunning(client, process) {
			runningAgentProcesses = append(runningAgentProcesses, process)
		}
	}
	return runningAgentProcesses, nil
}

// AgentProcessIsRunning returns true if the agent process is running
func AgentProcessIsRunning(client *TestClient, processName string) bool {
	running, err := process.IsRunning(client.Host, processName)
	return running && err == nil
}

// AssertPortBoundByService accepts a port and a service name and returns true if the port is bound by the service
func AssertPortBoundByService(t assert.TestingT, client *TestClient, transport string, port int, service string, processName string) (boundport.BoundPort, bool) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	pids, err := process.FindPID(client.Host, processName)
	if !assert.NoError(t, err) {
		return nil, false
	}
	if !assert.NotEmpty(t, pids, "service %s should be running", service) {
		fmt.Printf("service %s should be running\n", service)
		return nil, false
	}

	boundPort, err := GetBoundPort(client.Host, transport, port)
	if !assert.NoError(t, err) {
		return nil, false
	}
	if !assert.NotNil(t, boundPort, "port %s/%d should be bound", transport, port) {
		return nil, false
	}
	if !assert.Containsf(t, pids, boundPort.PID(), "port %#v should be bound by service %s", boundPort, service) {
		return boundPort, false
	}
	return boundPort, true
}

// GetBoundPort returns a port that is bound on the host, or nil if the port is not bound
func GetBoundPort(host *components.RemoteHost, transport string, port int) (boundport.BoundPort, error) {
	bports, err := boundport.BoundPorts(host)
	if err != nil {
		return nil, err
	}
	for _, bport := range bports {
		if bport.Transport() == transport && bport.LocalPort() == port {
			return bport, nil
		}
	}
	return nil, nil
}

// ReadJournalCtl returns the output of journalctl with an optional grep pattern
func ReadJournalCtl(t *testing.T, client *TestClient, grepPattern string) string {
	var cmd string
	if grepPattern != "" {
		cmd = fmt.Sprintf("journalctl | grep '%s'", grepPattern)
	} else {
		cmd = "journalctl"
	}
	t.Logf("Error encountered, getting the output of %s", cmd)
	journalCtlOutput, journalCtlErr := client.Host.Execute(cmd)
	if journalCtlErr != nil {
		t.Log("Skipping, journalctl failed to run")
	}
	return journalCtlOutput
}

// DockerTestClient is a helper to run commands on a docker container for tests
type DockerTestClient struct {
	host          *components.RemoteHost
	containerName string
}

// NewDockerTestClient creates a client to help write tests that run on a docker container
func NewDockerTestClient(host *components.RemoteHost, containerName string) *DockerTestClient {
	return &DockerTestClient{
		host:          host,
		containerName: containerName,
	}
}

// RunContainer starts the docker container in the background based on the given image reference
func (c *DockerTestClient) RunContainer(image string) error {
	// We run an infinite no-op to keep it alive
	_, err := c.host.Execute(
		fmt.Sprintf("docker run -d -e DD_HOSTNAME=docker-test --name '%s' '%s' tail -f /dev/null", c.containerName, image),
	)
	return err
}

// Cleanup force-removes the docker container associated to the client
func (c *DockerTestClient) Cleanup() error {
	_, err := c.host.Execute(fmt.Sprintf("docker rm -f '%s'", c.containerName))
	return err
}

// Execute runs commands on a Docker remote host
func (c *DockerTestClient) Execute(command string) (output string, err error) {
	return c.host.Execute(
		// Run command on container via docker exec and wrap with sh -c
		// to provide a similar interface to remote host's execute
		fmt.Sprintf("docker exec %s sh -c '%s'", c.containerName, command),
	)
}

// ExecuteWithRetry execute the command with retry
func (c *DockerTestClient) ExecuteWithRetry(cmd string) (output string, err error) {
	return execWithRetry(c.Execute, cmd)
}

func execWithRetry(exec func(string) (string, error), cmd string) (string, error) {
	var err error
	var output string
	maxTries := 5

	for try := 0; try < maxTries; try++ {
		output, err = exec(cmd)
		if err == nil {
			break
		}
		fmt.Printf("(attempt %d of %d) error while executing command in host: %v\n", try+1, maxTries, err)
		time.Sleep(time.Duration(math.Pow(2, float64(try))) * time.Second)
	}

	return output, err
}

// MacOSTestClient is a helper to run commands on a Mac OS remote host for tests
type MacOSTestClient struct {
	host *components.RemoteHost
}

// NewMacOSTestClient creates a client to help write tests that run on a Mac OS remote host
func NewMacOSTestClient(host *components.RemoteHost) *MacOSTestClient {
	return &MacOSTestClient{
		host: host,
	}
}

// Execute runs commands on a Mac OS remote host
func (c *MacOSTestClient) Execute(command string) (output string, err error) {
	return c.host.Execute(command)
}

// ExecuteWithRetry execute the command with retry
func (c *MacOSTestClient) ExecuteWithRetry(cmd string) (output string, err error) {
	return execWithRetry(c.Execute, cmd)
}
