// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dockerDaemonConfig map[string]interface{}
type dockerRuntimesConfig map[string]map[string]interface{}

const (
	tmpDockerDaemonPath = "/tmp/daemon.json.tmp"
	dockerDaemonPath    = "/etc/docker/daemon.json"
)

// setDockerConfig sets up the docker daemon to use the APM injector
// even if docker isn't installed, to prepare for if it is installed
// later
func (a *apmInjectorInstaller) setDockerConfig() error {
	// Create docker dir if it doesn't exist
	err := executeCommand(createDockerDirCommand)
	if err != nil {
		return err
	}

	dockerConfig := dockerDaemonConfig{}
	stat, err := os.Stat(dockerDaemonPath)
	if err == nil {
		// Read the existing configuration
		file, err := os.ReadFile(dockerDaemonPath)
		if err != nil {
			return err
		}
		err = json.Unmarshal(file, &dockerConfig)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	dockerConfig["default-runtime"] = "dd-shim"
	runtimes, ok := dockerConfig["runtimes"].(dockerRuntimesConfig)
	if !ok {
		runtimes = dockerRuntimesConfig{}
	}
	runtimes["dd-shim"] = map[string]interface{}{
		"path": path.Join(a.installPath, "inject", "auto_inject_runc"),
	}
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return err
	}

	// Write the new configuration to a temporary file
	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile(tmpDockerDaemonPath, dockerConfigJSON, perms)
	if err != nil {
		return err
	}

	// Move the temporary file to the final location
	err = executeCommand(string(replaceDockerCommand))
	if err != nil {
		return err
	}

	return restartDocker()
}

// deleteDockerConfig restores the docker daemon configuration
func (a *apmInjectorInstaller) deleteDockerConfig() error {
	dockerConfig := dockerDaemonConfig{}
	stat, err := os.Stat(dockerDaemonPath)
	if err == nil {
		// Read the existing configuration
		file, err := os.ReadFile(dockerDaemonPath)
		if err != nil {
			return err
		}
		err = json.Unmarshal(file, &dockerConfig)
		if err != nil {
			return err
		}
	} else if os.IsNotExist(err) {
		// If the file doesn't exist, there's nothing to do
		return nil
	}

	dockerConfig["default-runtime"] = "runc"
	runtimes, ok := dockerConfig["runtimes"].(dockerRuntimesConfig)
	if !ok {
		runtimes = dockerRuntimesConfig{}
	}
	delete(runtimes, "dd-shim")
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return err
	}

	// Write the new configuration to a temporary file
	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile(tmpDockerDaemonPath, dockerConfigJSON, perms)
	if err != nil {
		return err
	}

	// Move the temporary file to the final location
	err = executeCommand(string(replaceDockerCommand))
	if err != nil {
		return err
	}
	return restartDocker()
}

// restartDocker reloads the docker daemon if it exists
func restartDocker() error {
	if !isDockerInstalled() {
		log.Info("updater: docker is not installed, skipping reload")
		return nil
	}
	return executeCommand(restartDockerCommand)
}

// isDockerInstalled checks if docker is installed on the system
func isDockerInstalled() bool {
	cmd := exec.Command("which", "docker")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		log.Warn("updater: failed to check if docker is installed, assuming it isn't: ", err)
		return false
	}
	return len(outb.String()) != 0
}
