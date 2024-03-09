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
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

var (
	installPath string
	systemdPath = "/lib/systemd/system" // todo load it at build time from omnibus
)

func isValidChar(c rune) bool {
	return unicode.IsLower(c) || c == '.' || c == '-' || c == ' '
}

func isValidString(s string) bool {
	for _, c := range s {
		if !isValidChar(c) {
			return false
		}
	}
	return true
}

func buildCommand(inputCommand string) (*exec.Cmd, error) {
	if !isValidString(inputCommand) {
		return nil, fmt.Errorf("invalid command")
	}
	tokens := strings.Split(inputCommand, " ")

	if len(tokens) == 1 && tokens[0] == "systemd-reload" {
		return exec.Command("systemctl", "daemon-reload"), nil
	}
	if len(tokens) != 2 {
		return nil, fmt.Errorf("missing unit")
	}
	unit := tokens[1]
	if !strings.HasPrefix(unit, "datadog-") {
		return nil, fmt.Errorf("invalid unit")
	}

	command := tokens[0]
	switch command {
	case "stop", "enable", "disable":
		return exec.Command("systemctl", command, unit), nil
	case "start":
		// --no-block is used to avoid waiting on oneshot executions
		return exec.Command("systemctl", command, unit, "--no-block"), nil
	case "load-unit":
		return exec.Command("cp", installPath+"/systemd/"+unit, systemdPath+"/"+unit), nil
	case "remove-unit":
		return exec.Command("rm", systemdPath+"/"+unit), nil
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
