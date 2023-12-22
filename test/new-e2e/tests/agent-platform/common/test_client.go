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

	e2eClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	pkgmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/pkg-manager"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
	"gopkg.in/yaml.v2"
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
	ReadFile(path string) (string, error)
	FileExists(path string) (string, error)
	FindFileInFolder(path string) (string, error)
	WriteFile(path string, content string) (string, error)
}

// Helper generic interface
type Helper interface {
	GetInstallFolder() string
	GetConfigFolder() string
	GetBinaryPath() string
	GetConfigFileName() string
	GetServiceName() string
}

func getServiceManager(vmClient e2eClient.VM) ServiceManager {
	if _, err := vmClient.ExecuteWithError("command -v systemctl"); err == nil {
		return svcmanager.NewSystemctlSvcManager(vmClient)
	}

	if _, err := vmClient.ExecuteWithError("command -v /sbin/initctl"); err == nil {
		return svcmanager.NewUpstartSvcManager(vmClient)
	}

	if _, err := vmClient.ExecuteWithError("command -v service"); err == nil {
		return svcmanager.NewServiceSvcManager(vmClient)
	}
	return nil
}

func getPackageManager(vmClient e2eClient.VM) PackageManager {
	if _, err := vmClient.ExecuteWithError("command -v apt"); err == nil {
		return pkgmanager.NewAptPackageManager(vmClient)
	}

	if _, err := vmClient.ExecuteWithError("command -v yum"); err == nil {
		return pkgmanager.NewYumPackageManager(vmClient)
	}

	if _, err := vmClient.ExecuteWithError("command -v zypper"); err == nil {
		return pkgmanager.NewZypperPackageManager(vmClient)
	}

	return nil
}

// TestClient contain the Agent Env and SvcManager and PkgManager for tests
type TestClient struct {
	VMClient    e2eClient.VM
	AgentClient e2eClient.Agent
	Helper      Helper
	FileManager FileManager
	SvcManager  ServiceManager
	PkgManager  PackageManager
}

// NewTestClient create a an ExtendedClient from VMClient and AgentCommandRunner, includes svcManager and pkgManager to write agent-platform tests
func NewTestClient(vmClient e2eClient.VM, agentClient e2eClient.Agent, fileManager FileManager, helper Helper) *TestClient {
	svcManager := getServiceManager(vmClient)
	pkgManager := getPackageManager(vmClient)
	return &TestClient{
		VMClient:    vmClient,
		AgentClient: agentClient,
		Helper:      helper,
		FileManager: fileManager,
		SvcManager:  svcManager,
		PkgManager:  pkgManager,
	}
}

// CheckPortBound check if the port is currently bound, use netstat or ss
func (c *TestClient) CheckPortBound(port int) error {
	netstatCmd := "sudo netstat -lntp | grep %v"
	if _, err := c.VMClient.ExecuteWithError("command -v netstat"); err != nil {
		netstatCmd = "sudo ss -lntp | grep %v"
	}

	_, err := c.ExecuteWithRetry(fmt.Sprintf(netstatCmd, port))

	return err
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
	_, err = c.FileManager.WriteFile(confPath, string(confUpdated))
	return err
}

// GetPythonVersion returns python version from the Agent status
func (c *TestClient) GetPythonVersion() (string, error) {
	statusJSON := map[string]any{}
	ok := false
	var statusString string

	for try := 0; try < 60 && !ok; try++ {
		status, err := c.AgentClient.StatusWithError(e2eClient.WithArgs([]string{"-j"}))
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
		output, err := c.VMClient.ExecuteWithError("journalctl -u datadog-agent")
		if err != nil {
			fmt.Println("Failed to get logs from journalctl, ignoring... ")
		} else {
			fmt.Println("Logs from journalctl: ", output)
		}

		return "", err
	}
	pythonVersion := statusJSON["python_version"].(string)

	return pythonVersion, nil
}

// ExecuteWithRetry execute the command with retry
func (c *TestClient) ExecuteWithRetry(cmd string) (string, error) {
	ok := false

	var err error
	var output string

	for try := 0; try < 5 && !ok; try++ {
		output, err = c.VMClient.ExecuteWithError(cmd)
		if err == nil {
			ok = true
		}
		time.Sleep(1 * time.Second)
	}

	return output, err

}
