// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"encoding/json"
)

type unitCommand string

const (
	startCommand         unitCommand = "start"
	stopCommand          unitCommand = "stop"
	enableCommand        unitCommand = "enable"
	disableCommand       unitCommand = "disable"
	loadCommand          unitCommand = "load-unit"
	removeCommand        unitCommand = "remove-unit"
	systemdReloadCommand             = `{"command":"systemd-reload"}`
	adminExecutor                    = "datadog-updater-admin.service"
)

type privilegeCommand struct {
	Command string `json:"command,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Path    string `json:"path,omitempty"`
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
