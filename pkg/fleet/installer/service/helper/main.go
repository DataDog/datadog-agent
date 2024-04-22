// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a package that allows dd-agent
// to execute a subset of priviledged commands
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

var (
	installPath    string
	debSystemdPath = "/lib/systemd/system" // todo load it at build time from omnibus
	rpmSystemdPath = "/usr/lib/systemd/system"
	pkgDir         = "/opt/datadog-packages/"
	agentDir       = "/etc/datadog-agent"
	testSkipUID    = ""
)

// findSystemdPath todo: this is a hacky way to detect on which os family we are currently
// running and finding the correct systemd path.
// We should probably provide the correct path when we build the package
func findSystemdPath() (systemdPath string, err error) {
	if _, err = os.Stat(rpmSystemdPath); err == nil {
		return rpmSystemdPath, nil
	}
	if _, err = os.Stat(debSystemdPath); err == nil {
		return debSystemdPath, nil
	}
	return "", fmt.Errorf("systemd unit path error: %w", err)
}

func enforceUID() bool {
	return testSkipUID != "true"
}

type privilegeCommand struct {
	Command string `json:"command,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

func isValidUnitChar(c rune) bool {
	return unicode.IsLower(c) || c == '.' || c == '-'
}

func isValidUnitString(s string) bool {
	for _, c := range s {
		if !isValidUnitChar(c) {
			return false
		}
	}
	return true
}

func buildCommand(inputCommand privilegeCommand) (*exec.Cmd, error) {
	if inputCommand.Unit != "" {
		return buildUnitCommand(inputCommand)
	}

	if inputCommand.Path != "" {
		return buildPathCommand(inputCommand)
	}
	switch inputCommand.Command {
	case "systemd-reload":
		return exec.Command("systemctl", "daemon-reload"), nil
	case "agent-symlink":
		return exec.Command("ln", "-sf", "/opt/datadog-packages/datadog-agent/stable/bin/agent/agent", "/usr/bin/datadog-agent"), nil
	case "rm-agent-symlink":
		return exec.Command("rm", "-f", "/usr/bin/datadog-agent"), nil
	case "create-docker-dir":
		return exec.Command("mkdir", "-p", "/etc/docker"), nil
	case "replace-docker":
		return exec.Command("mv", filepath.Join(installPath, "run", "daemon.json.tmp"), "/etc/docker/daemon.json"), nil
	case "restart-docker":
		return exec.Command("systemctl", "restart", "docker"), nil
	case "replace-ld-preload":
		return exec.Command("mv", filepath.Join(installPath, "run", "ld.so.preload.tmp"), "/etc/ld.so.preload"), nil
	default:
		return nil, fmt.Errorf("invalid command")
	}
}

func buildUnitCommand(inputCommand privilegeCommand) (*exec.Cmd, error) {
	command := inputCommand.Command
	unit := inputCommand.Unit
	if !strings.HasPrefix(unit, "datadog-") || !isValidUnitString(unit) {
		return nil, fmt.Errorf("invalid unit")
	}
	switch command {
	case "stop", "enable", "disable":
		return exec.Command("systemctl", command, unit), nil
	case "start":
		// --no-block is used to avoid waiting on oneshot executions
		return exec.Command("systemctl", command, unit, "--no-block"), nil
	case "load-unit":
		systemdPath, err := findSystemdPath()
		if err != nil {
			return nil, err
		}
		return exec.Command("cp", filepath.Join(installPath, "systemd", unit), filepath.Join(systemdPath, unit)), nil
	case "remove-unit":
		systemdPath, err := findSystemdPath()
		if err != nil {
			return nil, err
		}
		return exec.Command("rm", filepath.Join(systemdPath, unit)), nil
	default:
		return nil, fmt.Errorf("invalid command")
	}
}

func buildPathCommand(inputCommand privilegeCommand) (*exec.Cmd, error) {
	path := inputCommand.Path
	// detects symlinks and ..
	absPath, err := filepath.Abs(path)
	if absPath != path || err != nil {
		return nil, fmt.Errorf("invalid path")
	}
	if !strings.HasPrefix(path, pkgDir) && !strings.HasPrefix(path, agentDir) {
		return nil, fmt.Errorf("invalid path")
	}
	switch inputCommand.Command {
	case "chown dd-agent":
		return exec.Command("chown", "-R", "dd-agent:dd-agent", path), nil
	case "rm":
		return exec.Command("rm", "-rf", path), nil
	case "backup-file":
		return exec.Command("cp", "-f", path, path+".bak"), nil
	case "restore-file":
		return exec.Command("mv", path+".bak", path), nil
	default:
		return nil, fmt.Errorf("invalid command")
	}
}

func executeCommand() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	inputCommand := os.Args[1]

	var pc privilegeCommand
	err := json.Unmarshal([]byte(inputCommand), &pc)
	if err != nil {
		return fmt.Errorf("decoding command %s", inputCommand)
	}
	command, err := buildCommand(pc)
	if err != nil {
		return err
	}

	commandErr := new(bytes.Buffer)
	command.Stderr = commandErr
	log.Printf("Running command: %s", command.String())
	err = command.Run()
	if err != nil {
		return fmt.Errorf("running command (%s): %s", err.Error(), commandErr.String())
	}
	return nil
}

func main() {
	err := setupPriviledges()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error setting up priviledges: %s\n", err)
		os.Exit(1)
	}
	log.SetOutput(os.Stdout)
	err = executeCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func setupPriviledges() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("only root can execute this command")
	}
	if err := syscall.Setuid(os.Getuid()); err != nil {
		return fmt.Errorf("failed to setuid: %s", err)
	}
	if err := syscall.Seteuid(0); err != nil {
		return fmt.Errorf("failed to seteuid: %s", err)
	}
	return nil
}
