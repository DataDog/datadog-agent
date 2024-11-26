// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package testutil holds different utilities and stubs for testing
package testutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetDockerContainerID returns the ID of a docker container.
func GetDockerContainerID(dockerName string) (string, error) {
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
