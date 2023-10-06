// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
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

	customEnv := os.Environ()
	for k, v := range c.Variables {
		customEnv = append(customEnv, fmt.Sprintf("%s=%s", k, v))
	}

	args := []string{
		"--project-name", c.ProjectName,
		"--file", c.FilePath,
	}
	pullCmd := exec.Command("docker-compose", append(args, "pull", "--parallel")...)
	pullCmd.Env = customEnv
	output, err := pullCmd.CombinedOutput()
	if err != nil {
		log.Errorf("fail to pull: %s %s", err, string(output))
		/*
			We retry once if we cannot pull the images, example:
			Pulling etcd (quay.io/coreos/etcd:latest)...
			ERROR: Get https://quay.io/v2/: net/http: request canceled (Client.Timeout exceeded while awaiting headers)
		*/
		log.Infof("retrying pull...")
		// We need to rebuild a new command because the file-descriptors of stdout/err are already set
		retryPull := exec.Command("docker-compose", append(args, "pull", "--parallel")...)
		retryPull.Env = customEnv
		output, err = retryPull.CombinedOutput()
		if err != nil {
			return output, err
		}
	}
	args = append(args, "up", "-d")

	if c.RemoveRebuildImages {
		args = append(args, "--build")
	}
	runCmd := exec.Command("docker-compose", args...)
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
	selfContainerID, err := metrics.GetProvider().GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return "host", nil
	}

	co, err := du.Inspect(context.TODO(), selfContainerID, false)
	if err != nil {
		return "host", nil
	}
	return fmt.Sprintf("container:%s", co.ID), nil
}
