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

var updaterHelper = filepath.Join(setup.InstallPath, "bin", "updater", "updater-helper")

// ChownDDAgent changes the owner of the given path to the dd-agent user.
func ChownDDAgent(path string) error {
	return executeCommand(`{"command":"chown dd-agent","path":"` + path + `"}`)
}

// RmPackageVersion removes the versioned files at a given path.
func RmPackageVersion(path string) error {
	return executeCommand(`{"command":"rm","path":"` + path + `"}`)
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
