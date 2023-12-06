// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	pkgmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/pkg-manager"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"testing"
)

// ServiceManager generic interface
type ServiceManager interface {
	Status(service string) (string, error)
	Start(service string) (string, error)
	Stop(service string) (string, error)
	Restart(service string) (string, error)
}

// PackageManager generic interface
type PackageManager interface {
	Remove(pkg string) (string, error)
}

// FileManager generic interface
type FileManager interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	FileExists(path string) (bool, error)
	WriteFile(path string, content []byte) (int64, error)
}

// Helper generic interface
type Helper interface {
	GetInstallFolder() string
	GetConfigFolder() string
	GetBinaryPath() string
	GetConfigFileName() string
	GetServiceName() string
	AgentProcesses() []string
}

func getServiceManager(host *components.RemoteHost) ServiceManager {
	if _, err := host.Execute("command -v systemctl"); err == nil {
		return svcmanager.NewSystemctlSvcManager(host)
	}

	if _, err := host.Execute("command -v /sbin/initctl"); err == nil {
		return svcmanager.NewUpstartSvcManager(host)
	}

	if _, err := host.Execute("command -v service"); err == nil {
		return svcmanager.NewServiceSvcManager(host)
	}
	return nil
}

func getPackageManager(host *components.RemoteHost) PackageManager {
	if _, err := host.Execute("command -v apt"); err == nil {
		return pkgmanager.NewAptPackageManager(host)
	}

	if _, err := host.Execute("command -v yum"); err == nil {
		return pkgmanager.NewYumPackageManager(host)
	}

	if _, err := host.Execute("command -v zypper"); err == nil {
		return pkgmanager.NewZypperPackageManager(host)
	}

	return nil
}

// TestClient contain the Agent Env and SvcManager and PkgManager for tests
type TestClient struct {
	Host        *components.RemoteHost
	AgentClient agentclient.Agent
	Helper      Helper
	FileManager FileManager
	SvcManager  ServiceManager
	PkgManager  PackageManager
}

// NewTestClient create a an ExtendedClient from VMClient and AgentCommandRunner, includes svcManager and pkgManager to write agent-platform tests
func NewTestClient(host *components.RemoteHost, agentClient agentclient.Agent, fileManager FileManager, helper Helper) *TestClient {
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

		// TEMPORARY DEBUG: on error print logs from journalctx
		output, err := c.Host.Execute("journalctl -u datadog-agent")
		if err != nil {
			fmt.Println("Failed to get logs from journalctl, ignoring... ")
		} else {
			fmt.Println("Logs from journalctl: ", output)
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
	fileManager := filemanager.NewClientFileManager(host)

	agentClient, err := client.NewHostAgentClient(t, host, false)
	require.NoError(t, err)

	helper := helpers.NewWindowsHelper()
	client := NewTestClient(host, agentClient, fileManager, helper)
	client.SvcManager = svcmanager.NewWindowsSvcManager(host)

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
	running, err := process.IsProcessRunning(client.Host, processName)
	return running && err == nil
}

// PortBoundByPID returns the info about the port bound by a given PID
func PortBoundByPID(client *TestClient, port int, pid int) (boundport.BoundPort, error) {
	ports, err := boundport.BoundPorts(client.Host)
	if err != nil {
		return nil, err
	}
	for _, boundPort := range ports {
		if boundPort.PID() == pid && boundPort.LocalPort() == port {
			return boundPort, nil
		}
	}
	return nil, nil
}

// PortBoundByService returns info about the port bound by a given service
func PortBoundByService(client *TestClient, port int, service string) (boundport.BoundPort, error) {
	// TODO: might need to map service name to process name, this is working right now though
	pids, err := process.FindPID(client.Host, service)
	if err != nil {
		return nil, err
	}

	for _, pid := range pids {
		boundPort, err := PortBoundByPID(client, port, pid)
		if err != nil {
			return nil, err
		}
		if boundPort != nil {
			return boundPort, nil
		}
	}
	return nil, nil
}
