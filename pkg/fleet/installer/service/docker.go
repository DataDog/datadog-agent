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
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type dockerDaemonConfig map[string]interface{}

var (
	dockerDaemonPath = "/etc/docker/daemon.json"
)

func (a *apmInjectorInstaller) setupDocker(ctx context.Context) (rollback func() error, err error) {
	if !isDockerInstalled(ctx) {
		return nil, nil
	}
	err = os.MkdirAll("/etc/docker", 0755)
	if err != nil {
		return nil, err
	}

	rollbackDockerConfig, err := a.dockerConfigInstrument.mutate()
	if err != nil {
		return nil, err
	}

	rollback = func() error {
		if err := rollbackDockerConfig(); err != nil {
			return err
		}
		return reloadDockerConfig(ctx)
	}
	return rollback, reloadDockerConfig(ctx)
}

func (a *apmInjectorInstaller) uninstallDocker(ctx context.Context) error {
	if !isDockerInstalled(ctx) {
		return nil
	}
	if _, err := a.dockerConfigUninstrument.mutate(); err != nil {
		return err
	}
	return reloadDockerConfig(ctx)
}

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

// verifyDockerConfig validates the docker daemon configuration for latest
// docker versions (>23.0.0)
func (a *apmInjectorInstaller) verifyDockerConfig(path string) error {
	// Get docker version
	cmd := exec.Command("docker", "version", "-f", "'{{.Client.Version}}'")
	versionBuffer := new(bytes.Buffer)
	cmd.Stdout = versionBuffer
	err := cmd.Run()
	if err != nil {
		log.Warnf("failed to get docker version: %s, skipping verification", err)
		return nil
	}
	majorDockerVersion := strings.Split(versionBuffer.String(), ".")[0]
	majorDockerVersionInt, err := strconv.Atoi(majorDockerVersion)
	if err != nil {
		log.Warnf("failed to parse docker version %s: %s, skipping verification", majorDockerVersion, err)
		return nil
	}
	if majorDockerVersionInt < 23 {
		log.Warnf("docker version %s is not supported for verification, skipping", majorDockerVersion)
		return nil
	}

	cmd = exec.Command("dockerd", "--validate", "--config-file", path)
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to validate docker config (%v): %s", err, buf.String())
	}
	return nil
}

func reloadDockerConfig(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "reload_docker")
	defer span.Finish(tracer.WithError(err))
	return exec.CommandContext(ctx, "systemctl", "reload", "docker").Run()
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
