// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

type unitCommand string

const (
	startCommand         = unitCommand("start")
	stopCommand          = unitCommand("stop")
	enableCommand        = unitCommand("enable")
	disableCommand       = unitCommand("disable")
	loadCommand          = unitCommand("load-unit")
	removeCommand        = unitCommand("remove-unit")
	systemdReloadCommand = "systemd-reload"
	adminExecutor        = "datadog-updater-admin.service"
)

var updaterHelper = filepath.Join(setup.InstallPath, "bin", "updater", "updater-helper")

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

func executeCommand(command string) error {
	cmd := exec.Command(updaterHelper, command)
	cmd.Stdout = os.Stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	stderrOutput, err := io.ReadAll(stderr)
	if err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return errors.New(string(stderrOutput))
	}
	return nil
}

func wrapUnitCommand(command unitCommand, unit string) string {
	return string(command) + " " + unit
}
