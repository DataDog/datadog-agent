// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"encoding/json"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type unitCommand string

var (
	systemdPath = "/lib/systemd/system" // todo load it at build time from omnibus
)

const (
	startCommand                 unitCommand = "start"
	stopCommand                  unitCommand = "stop"
	enableCommand                unitCommand = "enable"
	disableCommand               unitCommand = "disable"
	loadCommand                  unitCommand = "load-unit"
	removeCommand                unitCommand = "remove-unit"
	addInstallerToAgentGroup     unitCommand = "add-installer-to-agent-group"
	backupCommand                unitCommand = `backup-file`
	restoreCommand               unitCommand = `restore-file`
	replaceDockerCommand                     = `{"command":"replace-docker"}`
	restartDockerCommand                     = `{"command":"restart-docker"}`
	createDockerDirCommand                   = `{"command":"create-docker-dir"}`
	replaceLDPreloadCommand                  = `{"command":"replace-ld-preload"}`
	systemdReloadCommand                     = `{"command":"systemd-reload"}`
	seLinuxSetPermissionsCommand             = `{"command":"set-selinux-permissions"}`
	seLinuxRestoreContextCommand             = `{"command":"restore-selinux-context"}`
	adminExecutor                            = "datadog-updater-admin.service"
)

type privilegeCommand struct {
	Command string `json:"command,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// restartUnit restarts a systemd unit
func restartUnit(unit string) error {
	// check that the unit exists first
	if _, err := os.Stat(path.Join(systemdPath, unit)); os.IsNotExist(err) {
		log.Infof("Unit %s does not exist, skipping restart", unit)
		return nil
	}

	if err := stopUnit(unit); err != nil {
		return err
	}
	if err := startUnit(unit); err != nil {
		return err
	}
	return nil
}

func stopUnit(unit string) error {
	return executeCommand(wrapUnitCommand(stopCommand, unit))
}

func startUnit(unit string) error {
	return executeCommand(wrapUnitCommand(startCommand, unit))
}

func enableUnit(unit string) error {
	return executeCommand(wrapUnitCommand(enableCommand, unit))
}

func disableUnit(unit string) error {
	return executeCommand(wrapUnitCommand(disableCommand, unit))
}

func loadUnit(unit string) error {
	return executeCommand(wrapUnitCommand(loadCommand, unit))
}

func removeUnit(unit string) error {
	return executeCommand(wrapUnitCommand(removeCommand, unit))
}

func systemdReload() error {
	return executeCommand(systemdReloadCommand)
}

func wrapUnitCommand(command unitCommand, unit string) string {
	privilegeCommand := privilegeCommand{Command: string(command), Unit: unit}
	rawJSON, err := json.Marshal(privilegeCommand)
	if err != nil {
		// can't happen as we control the struct
		panic(err)
	}
	return string(rawJSON)
}

func executeCommandStruct(command privilegeCommand) error {
	rawJSON, err := json.Marshal(command)
	if err != nil {
		return err
	}
	privilegeCommandJSON := string(rawJSON)
	return executeCommand(privilegeCommandJSON)
}
