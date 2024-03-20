// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a package that allows dd-updater
// to execute a subset of priviledged commands
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

var (
	installPath        string
	systemdPath        = "/lib/systemd/system" // todo load it at build time from omnibus
	updaterInstallPath = "/opt/datadog-packages/datadog-updater"
)

func isValidChar(c rune) bool {
	return unicode.IsLower(c) || ('0' <= c && c <= '9') || c == '.' || c == '-' || c == ' '
}

func isValidString(s string) bool {
	for _, c := range s {
		if !isValidChar(c) {
			return false
		}
	}
	return true
}

func parseUnitCmd(tokens []string) (unit string, err error) {
	if len(tokens) != 2 {
		return "", fmt.Errorf("missing unit")
	}
	unit = tokens[1]
	if !strings.HasPrefix(unit, "datadog-") {
		return "", fmt.Errorf("invalid unit")
	}
	return unit, nil
}

func parseSetCapCmd(tokens []string) (target string, err error) {
	if len(tokens) != 2 {
		return "", fmt.Errorf("missing target")
	}
	target = tokens[1]
	if target == "stable" || target == "experiment" {
		return "", fmt.Errorf("target should be a concrete package version")
	}

	// sanitize the target to be a single path section
	for i, c := range target {
		if c == '.' || os.IsPathSeparator(target[i]) {
			return "", fmt.Errorf("target should not contain character like . and /")
		}
	}
	return target, nil
}

func buildCommand(inputCommand string) (*exec.Cmd, error) {
	if !isValidString(inputCommand) {
		return nil, fmt.Errorf("invalid command")
	}
	tokens := strings.Split(inputCommand, " ")
	if len(tokens) == 0 {
		return nil, fmt.Errorf("invalid command")
	}

	command := tokens[0]

	if command == "systemd-reload" && len(tokens) == 1 {
		return exec.Command("systemctl", "daemon-reload"), nil
	}

	if command == "set-updater-helper-capabilities" {
		target, err := parseSetCapCmd(tokens)
		if err != nil {
			return nil, err
		}
		updaterHelperPath := filepath.Join(updaterInstallPath, target, "bin/updater/updater-helper")
		info, err := os.Stat(updaterHelperPath)
		if err != nil {
			return nil, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("couldn't get update helper stats: %w", err)
		}
		// If the updater_helper is owned by root, don't do any check
		// this is necessary because the initial bootstrap will create packages from root
		if stat.Uid != 0 {
			// TODO(paul): change this to dd-updater user once the new user is introduced
			ddAgentUser, err := user.Lookup("dd-agent")
			if err != nil {
				return nil, fmt.Errorf("failed to lookup dd-agent user: %s", err)
			}
			if ddAgentUser.Uid != strconv.Itoa(int(stat.Uid)) {
				return nil, fmt.Errorf("updater-helper should be owned by dd-agent")
			}
		}

		if stat.Mode != 750 {
			return nil, fmt.Errorf("updater-helper should only be executable by the user")
		}

		return exec.Command("setcap", "cap_setuid+ep", updaterHelperPath), nil
	}

	unit, err := parseUnitCmd(tokens)
	if err != nil {
		return nil, err
	}
	switch command {
	case "stop", "enable", "disable":
		return exec.Command("systemctl", command, unit), nil
	case "start":
		// --no-block is used to avoid waiting on oneshot executions
		return exec.Command("systemctl", command, unit, "--no-block"), nil
	case "load-unit":
		return exec.Command("cp", filepath.Join(installPath, "systemd", unit), filepath.Join(systemdPath, unit)), nil
	case "remove-unit":
		return exec.Command("rm", filepath.Join(systemdPath, unit)), nil
	default:
		return nil, fmt.Errorf("invalid command")
	}

}

func executeCommand() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	inputCommand := os.Args[1]

	currentUser := syscall.Getuid()
	log.Printf("Current user: %d", currentUser)

	command, err := buildCommand(inputCommand)
	if err != nil {
		return err
	}

	// only root or dd-agent can execute this command
	if currentUser != 0 {
		ddAgentUser, err := user.Lookup("dd-agent")
		if err != nil {
			return fmt.Errorf("failed to lookup dd-agent user: %s", err)
		}
		if strconv.Itoa(currentUser) != ddAgentUser.Uid {
			return fmt.Errorf("only root or dd-agent can execute this command")
		}
	}

	log.Printf("Setting to root user")
	if err := syscall.Setuid(0); err != nil {
		return fmt.Errorf("failed to setuid: %s", err)
	}
	defer func() {
		log.Printf("Setting back to current user: %d", currentUser)
		err := syscall.Setuid(currentUser)
		if err != nil {
			log.Printf("Failed to set back to current user: %s", err)
		}
	}()

	log.Printf("Running command: %s", command.String())
	return command.Run()
}

func main() {
	log.SetOutput(os.Stdout)
	err := executeCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
