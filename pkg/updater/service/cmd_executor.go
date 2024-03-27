// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

var updaterHelper = filepath.Join(setup.InstallPath, "bin", "updater", "updater-helper")

// ChownDDAgent changes the owner of the given path to the dd-agent user.
func ChownDDAgent(path string) error {
	return executeCommand(`{"command":"chown dd-agent","path":"` + path + `"}`)
}

// RemoveAll removes all files under a given path under /opt/datadog-packages regardless of their owner.
func RemoveAll(path string) error {
	return executeCommand(`{"command":"rm","path":"` + path + `"}`)
}

func createAgentSymlink() error {
	return executeCommand(`{"command":"agent-symlink"}`)
}

func rmAgentSymlink() error {
	return executeCommand(`{"command":"rm-agent-symlink"}`)
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

// BuildHelperForTests builds the updater-helper binary for test
func BuildHelperForTests(pkgDir, binPath string, skipUIDCheck bool) error {
	updaterHelper = filepath.Join(binPath, "/updater-helper")
	localPath, _ := filepath.Abs(".")
	targetDir := "datadog-agent/pkg"
	index := strings.Index(localPath, targetDir)
	pkgPath := localPath[:index+len(targetDir)]
	helperPath := filepath.Join(pkgPath, "updater", "service", "helper", "main.go")
	cmd := exec.Command("go", "build", fmt.Sprintf(`-ldflags=-X main.pkgDir=%s -X main.testSkipUID=%v`, pkgDir, skipUIDCheck), "-o", updaterHelper, helperPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
