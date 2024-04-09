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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector() error {
	var err error
	defer func() {
		if err != nil {
			removeErr := RemoveAPMInjector()
			if removeErr != nil {
				log.Warnf("Failed to remove APM injector: %v", removeErr)
			}
		}
	}()

	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"
	err = os.Chmod(path.Join(injectorPath, "inject", "run"), 0777)
	if err != nil {
		return err
	}

	err = setupLDPreload(injectorPath)
	if err != nil {
		return err
	}

	err = setupDockerDaemon(injectorPath)
	if err != nil {
		return err
	}

	return nil
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector() error {
	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"
	err := removeLDPreload()
	if err != nil {
		return err
	}

	err = removeDockerDaemon(injectorPath)
	if err != nil {
		return err
	}
	return nil
}

// setupLDPreload adds preload options on /etc/ld.so.preload, overriding existing ones
func setupLDPreload(basePath string) error {
	launcherPreloadPath := path.Join(basePath, "inject", "launcher.preload.so")
	ldSoPreloadPath := "/etc/ld.so.preload"
	var ldSoPreload []byte
	if _, err := os.Stat(ldSoPreloadPath); err == nil {
		ldSoPreload, err = os.ReadFile(ldSoPreloadPath)
		if err != nil {
			return err
		}
		if strings.Contains(string(ldSoPreload), launcherPreloadPath) {
			// If the line of interest is already in /etc/ld.so.preload, return fast
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeLDPreload()
}

// removeLDPreload removes the preload options from /etc/ld.so.preload
func removeLDPreload() error {
	return executeCommand(string(removeLdPreloadCommand))
}

// writeLDPreload writes the content to /etc/ld.so.preload
func writeLDPreload() error {
	return executeCommand(string(setupLdPreloadCommand))
}

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
	return executeCommand(backupDockerCommand)
}

// restoreDockerDaemon restores the docker daemon configuration
func restoreDockerDaemon() error {
	return executeCommand(restoreDockerCommand)
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
	return executeCommand(reloadDockerCommand)
}
