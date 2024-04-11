// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"os"
	"os/exec"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setupDockerDaemon sets up the docker daemon to use the APM injector
// if docker is detected on the system
func setupDockerDaemon(basePath string) error {
	if !isDockerInstalled() {
		return nil
	}

	// Backup the docker daemon configuration
	err := backupDockerDaemon()
	if err != nil {
		return err
	}

	// Link the docker daemon configuration to the APM injector's
	err = linkDockerDaemon(path.Join(basePath, "daemon.json"))
	if err != nil {
		return err
	}

	return reloadDocker()
}

// removeDockerDaemon restores the docker daemon configuration
func removeDockerDaemon(basePath string) error {
	if !isDockerInstalled() {
		return nil
	}

	// Check backup exists if yes uses it, else use default configuration
	_, err := os.Stat("/etc/docker/daemon.json.bak")
	if err != nil && os.IsNotExist(err) {
		err = cleanupDockerDaemon(path.Join(basePath, "daemon-cleanup.json"))
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		err := restoreDockerDaemon()
		if err != nil {
			return err
		}
	}

	return reloadDocker()
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

// backupDockerDaemon backs up the docker daemon configuration
func backupDockerDaemon() error {
	_, err := os.Stat("/etc/docker/daemon.json.bak")
	if err == nil {
		return nil // Already backed up, fail fast
	}
	return executeCommandStruct(privilegeCommand{
		Command: string(backupCommand),
		Path:    "/etc/docker/daemon.json",
	})
}

// restoreDockerDaemon restores the docker daemon configuration
func restoreDockerDaemon() error {
	return executeCommandStruct(privilegeCommand{
		Command: string(restoreCommand),
		Path:    "/etc/docker/daemon.json",
	})
}

// linkDockerDaemon links the docker daemon configuration to the APM injector's
func linkDockerDaemon(path string) error {
	return executeCommandStruct(privilegeCommand{
		Command: string(linkDockerCommand),
		Path:    path,
	})
}

// cleanupDockerDaemon cleans up the docker daemon configuration using the default
func cleanupDockerDaemon(path string) error {
	return executeCommandStruct(privilegeCommand{
		Command: string(cleanupDockerCommand),
		Path:    path,
	})
}

// reloadDocker reloads the docker daemon
func reloadDocker() error {
	return executeCommand(restartDockerCommand)
}
