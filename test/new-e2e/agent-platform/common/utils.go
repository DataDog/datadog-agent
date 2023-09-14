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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	e2eClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"gopkg.in/yaml.v2"
)

// ServiceManager generic interface
type ServiceManager interface {
	Status(service string) (string, error)
	Start(service string) (string, error)
	Stop(service string) (string, error)
	Restart(service string) (string, error)
}

// NewClientFromEnv create a an ExtendedClient from Agent Env, includes svcManager and pkgManager to write agent-platform tests
func NewClientFromEnv(env *e2e.AgentEnv) *ExtendedClient {
	svcManager := getServiceManager(env)
	pkgManager := getPackageManager(env)
	return &ExtendedClient{
		Env:        env,
		SvcManager: svcManager,
		PkgManager: pkgManager,
	}
}

func getServiceManager(env *e2e.AgentEnv) ServiceManager {
	if _, err := env.VM.ExecuteWithError("systemctl --version"); err == nil {
		return &SystemCtlSvcManager{env}
	}
	return nil
}

func getPackageManager(env *e2e.AgentEnv) PackageManager {
	if _, err := env.VM.ExecuteWithError("apt-get --version"); err == nil {
		return &AptPackageManager{env}
	}
	return nil
}

// PackageManager generic interface
type PackageManager interface {
	Remove(pkg string) (string, error)
}

// ExtendedClient contain the Agent Env and SvcManager and PkgManager for tests
type ExtendedClient struct {
	Env        *e2e.AgentEnv
	SvcManager ServiceManager
	PkgManager PackageManager
}

// SystemCtlSvcManager struct for the Systemctl service manager
type SystemCtlSvcManager struct {
	env *e2e.AgentEnv
}

// Status returns status from systemctl
func (s *SystemCtlSvcManager) Status(service string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("systemctl status --no-pager %s.service", service))
}

// Stop executes stop command from stystemctl
func (s *SystemCtlSvcManager) Stop(service string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("sudo systemctl stop %s.service", service))
}

// Start executes start command from systemctl
func (s *SystemCtlSvcManager) Start(service string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("sudo systemctl start %s.service", service))
}

// Restart executes restart command from systemctl
func (s *SystemCtlSvcManager) Restart(service string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("sudo systemctl restart %s.service", service))
}

// AptPackageManager struct for Apt package manager
type AptPackageManager struct {
	env *e2e.AgentEnv
}

// Remove call remove from apt
func (s *AptPackageManager) Remove(pkg string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("sudo apt-get remove -q -y %s", pkg))
}

// CheckPortBound check if the port is currently bound, use netstat or ss
func (c *ExtendedClient) CheckPortBound(port int) error {
	netstatCmd := "sudo netstat -lntp | grep %v"
	if _, err := c.Env.VM.ExecuteWithError("sudo netstat --version"); err != nil {
		netstatCmd = "sudo ss -lntp | grep %v"
	}

	ok := false
	var err error

	for try := 0; try < 5 && !ok; try++ {
		_, err = c.Env.VM.ExecuteWithError(fmt.Sprintf(netstatCmd, port))
		if err == nil {
			ok = true
		}
		time.Sleep(1 * time.Second)
	}

	return err
}

// SetConfig set config given a key and a path to a yaml config file, support key nested twice at most
func (c *ExtendedClient) SetConfig(confPath string, key string, value string) error {
	confYaml := map[string]any{}
	conf, err := c.Env.VM.ExecuteWithError(fmt.Sprintf("cat %s", confPath))
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
			confYaml[keyList[0]].(map[string]any)[keyList[1]] = value
		}
	}

	confUpdated, err := yaml.Marshal(confYaml)
	if err != nil {
		return err
	}
	c.Env.VM.Execute(fmt.Sprintf(`sudo bash -c " echo '%s' > %s"`, confUpdated, confPath))
	return nil
}

// GetPythonVersion returns python version from the Agent status
func (c *ExtendedClient) GetPythonVersion() (string, error) {
	statusJSON := map[string]any{}
	ok := false
	var statusString string

	for try := 0; try < 5 && !ok; try++ {
		status, err := c.Env.Agent.StatusWithError(e2eClient.WithArgs([]string{"-j"}))
		if err == nil {
			ok = true
			statusString = status.Content
		}
		time.Sleep(1 * time.Second)
	}

	err := json.Unmarshal([]byte(statusString), &statusJSON)
	if err != nil {
		return "", err
	}
	pythonVersion := statusJSON["python_version"].(string)

	return pythonVersion, nil
}
