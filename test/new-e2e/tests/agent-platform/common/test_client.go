// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	boundport "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	pkgmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/pkg-manager"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"testing"
)

type tHelper interface {
	Helper()
}

func getServiceManager(host *components.RemoteHost) svcmanager.ServiceManager {
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
	svcManager := getServiceManager(host)
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

// SetConfig set config given a key and a path to a yaml config file, support key nested twice at most
func (c *TestClient) SetConfig(confPath string, key string, value string) error {
	confYaml := map[string]any{}
	conf, err := c.FileManager.ReadFile(confPath)
	if err != nil {
		fmt.Printf("config file: %s not found, it will be created\n", confPath)
	}
	if err := yaml.Unmarshal([]byte(conf), &confYaml); err != nil {
		return err
	}
	keyList := strings.Split(key, ".")

	if len(keyList) == 1 {
		confYaml[keyList[0]] = value
	}
	if len(keyList) == 2 {
		if confYaml[keyList[0]] == nil {
			confYaml[keyList[0]] = map[string]any{keyList[1]: value}
		} else {
			confYaml[keyList[0]].(map[interface{}]any)[keyList[1]] = value
		}
	}

	confUpdated, err := yaml.Marshal(confYaml)
	if err != nil {
		return err
	}
	_, err = c.FileManager.WriteFile(confPath, confUpdated)
	return err
}

func (c *TestClient) getJSONStatus() (map[string]any, error) {
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
	statusJSON, err := c.getJSONStatus()
	if err != nil {
		return "", err
	}
	pythonVersion := statusJSON["python_version"].(string)

	return pythonVersion, nil
}

// GetAgentVersion returns agent version from the Agent status
func (c *TestClient) GetAgentVersion() (string, error) {
	statusJSON, err := c.getJSONStatus()
	if err != nil {
		return "", err
	}
	agentVersion := statusJSON["version"].(string)

	return agentVersion, nil
}

// ExecuteWithRetry execute the command with retry
func (c *TestClient) ExecuteWithRetry(cmd string) (string, error) {
	ok := false

	var err error
	var output string

	for try := 0; try < 5 && !ok; try++ {
		output, err = c.Host.Execute(cmd)
		if err == nil {
			ok = true
		}
		time.Sleep(1 * time.Second)
	}

	return output, err
}

// NewWindowsTestClient create a TestClient for Windows VM
func NewWindowsTestClient(t *testing.T, host *components.RemoteHost) *TestClient {
	fileManager := filemanager.NewRemoteHost(host)

	agentClient, err := client.NewHostAgentClient(t, host, false)
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
func AssertPortBoundByService(t assert.TestingT, client *TestClient, port int, service string) (boundport.BoundPort, bool) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	// TODO: might need to map service name to process name, this is working right now though
	pids, err := process.FindPID(client.Host, service)
	if !assert.NoError(t, err) {
		return nil, false
	}
	if !assert.NotEmpty(t, pids, "service %s should be running", service) {
		return nil, false
	}

	boundPort, err := GetBoundPort(client.Host, port)
	if !assert.NoError(t, err) {
		return nil, false
	}
	if !assert.NotNil(t, boundPort, "port %d should be bound", port) {
		return nil, false
	}
	if !assert.Containsf(t, pids, boundPort.PID(), "port %#v should be bound by service %s", boundPort, service) {
		return boundPort, false
	}
	return boundPort, true
}

// GetBoundPort returns a port that is bound on the host, or nil if the port is not bound
func GetBoundPort(host *components.RemoteHost, port int) (boundport.BoundPort, error) {
	ports, err := boundport.BoundPorts(host)
	if err != nil {
		return nil, err
	}
	for _, boundPort := range ports {
		if boundPort.LocalPort() == port {
			return boundPort, nil
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
