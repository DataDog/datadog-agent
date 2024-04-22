// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
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
	startCommand            unitCommand = "start"
	stopCommand             unitCommand = "stop"
	enableCommand           unitCommand = "enable"
	disableCommand          unitCommand = "disable"
	loadCommand             unitCommand = "load-unit"
	removeCommand           unitCommand = "remove-unit"
	backupCommand           unitCommand = `backup-file`
	restoreCommand          unitCommand = `restore-file`
	replaceDockerCommand                = `{"command":"replace-docker"}`
	restartDockerCommand                = `{"command":"restart-docker"}`
	createDockerDirCommand              = `{"command":"create-docker-dir"}`
	replaceLDPreloadCommand             = `{"command":"replace-ld-preload"}`
	systemdReloadCommand                = `{"command":"systemd-reload"}`
)

type privilegeCommand struct {
	Command string `json:"command,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// restartUnit restarts a systemd unit
func restartUnit(ctx context.Context, unit string) error {
	// check that the unit exists first
	if _, err := os.Stat(path.Join(systemdPath, unit)); os.IsNotExist(err) {
		log.Infof("Unit %s does not exist, skipping restart", unit)
		return nil
	}

	if err := stopUnit(ctx, unit); err != nil {
		return err
	}
	if err := startUnit(ctx, unit); err != nil {
		return err
	}
	return nil
}

func stopUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(stopCommand, unit))
}

func startUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(startCommand, unit))
}

func enableUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(enableCommand, unit))
}

func disableUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(disableCommand, unit))
}

func loadUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(loadCommand, unit))
}

func removeUnit(ctx context.Context, unit string) error {
	return executeHelperCommand(ctx, wrapUnitCommand(removeCommand, unit))
}

func systemdReload(ctx context.Context) error {
	return executeHelperCommand(ctx, systemdReloadCommand)
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

func executeCommandStruct(ctx context.Context, command privilegeCommand) error {
	rawJSON, err := json.Marshal(command)
	if err != nil {
		return err
	}
	privilegeCommandJSON := string(rawJSON)
	return executeHelperCommand(ctx, privilegeCommandJSON)
}

// IsSystemdRunning checks if systemd is running using the documented way
// https://www.freedesktop.org/software/systemd/man/latest/sd_booted.html#Notes
func IsSystemdRunning() (running bool, err error) {
	_, err = os.Stat("/run/systemd/system")
	if os.IsNotExist(err) {
		log.Infof("Installer: systemd is not running, skip unit setup")
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
