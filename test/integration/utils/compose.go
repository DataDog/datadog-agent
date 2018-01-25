// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

type ComposeConf struct {
	ProjectName         string
	FilePath            string
	Variables           map[string]string
	NetworkMode         string // will provide $network_mode
	RemoveRebuildImages bool
}

// Start runs a docker-compose configuration
// All environment variables are propagated to the compose as $variable
// $network_mode is automatically set if empty
func (c *ComposeConf) Start() ([]byte, error) {
	var err error

	if c.NetworkMode == "" {
		c.NetworkMode, err = getNetworkMode()
		if err != nil {
			return nil, err
		}
	}
	if len(c.Variables) == 0 {
		// be sure we have an allocated map
		c.Variables = map[string]string{}
	}

	c.Variables["network_mode"] = c.NetworkMode
	args := []string{
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
		"up",
		"-d",
	}
	if c.RemoveRebuildImages {
		args = append(args, "--build")
	}
	runCmd := exec.Command("docker-compose", args...)

	customEnv := os.Environ()
	for k, v := range c.Variables {
		customEnv = append(customEnv, fmt.Sprintf("%s=%s", k, v))
	}
	runCmd.Env = customEnv

	return runCmd.CombinedOutput()
}

// Stop stops a running docker-compose configuration
func (c *ComposeConf) Stop() ([]byte, error) {
	args := []string{
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
		"down",
	}
	if c.RemoveRebuildImages {
		args = append(args, "--rmi", "all")
	}
	runCmd := exec.Command("docker-compose", args...)
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

// getNetworkMode provide a way to feed docker-compose network_mode
func getNetworkMode() (string, error) {
	du, err := docker.GetDockerUtil()
	if err != nil {
		return "", err
	}

	// Get container id if containerized
	co, err := du.InspectSelf()
	if err != nil {
		return "host", nil
	}
	return fmt.Sprintf("container:%s", co.ID), nil
}
