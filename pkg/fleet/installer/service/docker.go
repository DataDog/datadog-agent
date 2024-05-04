// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type dockerDaemonConfig map[string]interface{}

var (
	dockerDaemonPath = "/etc/docker/daemon.json"
)

// setDockerConfigContent sets the content of the docker daemon configuration
func (a *apmInjectorInstaller) setDockerConfigContent(previousContent []byte) ([]byte, error) {
	dockerConfig := dockerDaemonConfig{}

	if len(previousContent) > 0 {
		err := json.Unmarshal(previousContent, &dockerConfig)
		if err != nil {
			return nil, err
		}
	}

	dockerConfig["default-runtime"] = "dd-shim"
	runtimes, ok := dockerConfig["runtimes"].(map[string]interface{})
	if !ok {
		runtimes = map[string]interface{}{}
	}
	runtimes["dd-shim"] = map[string]interface{}{
		"path": path.Join(a.installPath, "inject", "auto_inject_runc"),
	}
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return nil, err
	}

	return dockerConfigJSON, nil
}

// deleteDockerConfigContent restores the content of the docker daemon configuration
func (a *apmInjectorInstaller) deleteDockerConfigContent(previousContent []byte) ([]byte, error) {
	dockerConfig := dockerDaemonConfig{}

	if len(previousContent) > 0 {
		err := json.Unmarshal(previousContent, &dockerConfig)
		if err != nil {
			return nil, err
		}
	}

	if defaultRuntime, ok := dockerConfig["default-runtime"].(string); ok && defaultRuntime == "dd-shim" || !ok {
		dockerConfig["default-runtime"] = "runc"
	}
	runtimes, ok := dockerConfig["runtimes"].(map[string]interface{})
	if !ok {
		runtimes = map[string]interface{}{}
	}
	delete(runtimes, "dd-shim")
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return nil, err
	}

	return dockerConfigJSON, nil
}

// restartDocker reloads the docker daemon if it exists
func restartDocker(ctx context.Context) error {
	span, _ := tracer.StartSpanFromContext(ctx, "restart_docker")
	defer span.Finish()
	if !isDockerInstalled(ctx) {
		log.Info("installer: docker is not installed, skipping reload")
		return nil
	}
	return exec.CommandContext(ctx, "systemctl", "restart", "docker").Run()
}

// isDockerInstalled checks if docker is installed on the system
func isDockerInstalled(ctx context.Context) bool {
	span, _ := tracer.StartSpanFromContext(ctx, "is_docker_installed")
	defer span.Finish()
	cmd := exec.CommandContext(ctx, "which", "docker")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	span.SetTag("is_installed", err == nil)
	if err != nil {
		log.Warn("installer: failed to check if docker is installed, assuming it isn't: ", err)
		return false
	}
	return len(outb.String()) != 0
}
