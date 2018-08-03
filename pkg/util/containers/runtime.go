// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// This code is not tied to docker itself, hence no docker build flag.
// It could be moved to its own package.

// +build linux

package containers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// Return values per container runtime
const (
	RuntimeNameDocker     string = "docker"
	RuntimeNameContainerd string = "containerd"
	RuntimeNameCRIO       string = "cri-o"
)

// Internal constants
const (
	daemonNameDockerLegacy1  string = "dockerd"
	daemonNameDockerLegacy2  string = "dockerd-current" // CentOS
	shimNameContainerd       string = "containerd-shim"
	shimNameCRIO             string = "conmon"
	shimNameContainerdUnsure string = "docker-containerd-shim"
	shimArgContainerdK8s     string = "-namespace k8s.io"
	shimArgContainerdDocker  string = "-namespace moby"
)

// ErrNoRuntimeMatch is returned when no container runtime can be matched
var ErrNoRuntimeMatch = errors.New("cannot match a container runtime")

// ErrNoContainerMatch is returned when no container ID can be matched
var ErrNoContainerMatch = errors.New("cannot match a container ID")

// EntityForPID returns the entity ID for a given PID. It can return
// either ErrNoRuntimeMatch or ErrNoContainerMatch if no match its
// found for the PID.
func EntityForPID(pid int32) (string, error) {
	cID, err := metrics.ContainerIDForPID(int(pid))
	if err != nil {
		return "", err
	}
	if cID == "" {
		return "", ErrNoContainerMatch
	}

	runtime, err := GetRuntimeForPID(pid)
	if err != nil {
		return "", err
	}
	value := fmt.Sprintf("%s://%s", runtime, cID)
	return value, nil
}

// GetRuntimeForPID inspects a PID's parents to detect a container runtime.
// For now, this assumes we are running on hostPID, as gopsutil looks-up
// processess in `/proc` (or HOST_PROC if set)
func GetRuntimeForPID(pid int32) (string, error) {
	var currentProcess *process.Process
	// Inspect given process
	currentProcess, err := process.NewProcess(pid)
	if err != nil {
		return "", err
	}

	for {
		// Get process cmdline and extract cmd name. Not using the `exe`
		// symlink because we don't always have permissions to read it
		cmdline, err := currentProcess.CmdlineSlice()
		if err != nil {
			return "", err
		}
		cmd := cmdline[0]
		if strings.Contains(cmd, "/") {
			cmdParts := strings.Split(cmd, "/")
			cmd = cmdParts[len(cmdParts)-1]
		}
		// Match with supported shim names
		switch cmd {
		case shimNameContainerdUnsure:
			// Shim can be used either by k8s for direct containerd
			// or new docker versions, checking arguments
			args := strings.Join(cmdline[1:], " ")
			switch {
			case strings.Contains(args, shimArgContainerdK8s):
				return RuntimeNameContainerd, nil
			case strings.Contains(args, shimArgContainerdDocker):
				return RuntimeNameDocker, nil
			}
		case shimNameContainerd:
			return RuntimeNameContainerd, nil
		case shimNameCRIO:
			return RuntimeNameCRIO, nil
		case daemonNameDockerLegacy1:
			return RuntimeNameDocker, nil
		case daemonNameDockerLegacy2:
			return RuntimeNameDocker, nil
		}

		// Didn't match, are we at PID 1 yet?
		if currentProcess.Pid == 1 {
			return "", ErrNoRuntimeMatch
		}
		// Else, go up to parent process and loop
		currentProcess, err = currentProcess.Parent()
		if err != nil {
			return "", err
		}
	}
}
