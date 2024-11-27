// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package docker provides API to manage docker/docker-compose lifecycle in UTs
package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GetMainPID returns the PID of the main process in the docker container.
func GetMainPID(dockerName string) (int64, error) {
	// Ensuring no previous instances exists.
	c := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", dockerName)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return 0, fmt.Errorf("failed to get %s pid: %s", dockerName, stderr.String())
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(stdout.String()), 10, 64)
	if pid == 0 {
		return 0, fmt.Errorf("failed to retrieve %s pid, container is not running", dockerName)
	}
	return pid, err
}

// GetContainerID returns the ID of a docker container.
func GetContainerID(dockerName string) (string, error) {
	// Ensuring no previous instances exists.
	c := exec.Command("docker", "inspect", "-f", "{{.Id}}", dockerName)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("failed to get %s ID: %s", dockerName, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
