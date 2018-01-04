// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type ComposeConf struct {
	ProjectName string
	FilePath    string
	Variables   map[string]string
}

// Start runs a docker-compose configuration
func (c *ComposeConf) Start() ([]byte, error) {
	runCmd := exec.Command(
		"docker-compose",
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
		"up", "-d")
	if len(c.Variables) > 0 {
		customEnv := os.Environ()
		for k, v := range c.Variables {
			customEnv = append(customEnv, fmt.Sprintf("%s=%s", k, v))
		}
		runCmd.Env = customEnv
	}
	return runCmd.CombinedOutput()
}

// Stop stops a running docker-compose configuration
func (c *ComposeConf) Stop() ([]byte, error) {
	runCmd := exec.Command(
		"docker-compose",
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
		"down")
	return runCmd.CombinedOutput()
}

// ListContainers lists the running container IDs
func (c *ComposeConf) ListContainers() ([]string, error) {
	runCmd := exec.Command(
		"docker-compose",
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
		"ps", "-q")

	out, err := runCmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var containerIDs []string

	for _, line := range lines {
		if len(strings.TrimSpace(line)) > 0 {
			containerIDs = append(containerIDs, strings.TrimSpace(line))
		}
	}
	return containerIDs, nil
}
