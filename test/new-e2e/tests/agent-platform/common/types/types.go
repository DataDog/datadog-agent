// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides types for uses in agent-platform tests
package types

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
)

// Executor is used to execute arbitrary commands
type Executor interface {
	Execute(command string) (output string, err error)
}

// A Host groups data and operations on a host
type Host struct {
	Executor
	OSFamily componentos.Family
}

// NewHostFromRemote creates a Host instance from a components.RemoteHost
func NewHostFromRemote(host *components.RemoteHost) *Host {
	executor := remoteHostExecutor(*host)
	return &Host{
		Executor: &executor,
		OSFamily: host.OSFamily,
	}
}

type remoteHostExecutor components.RemoteHost

// Execute runs commands to satisfy the Executor interface
func (e *remoteHostExecutor) Execute(command string) (output string, err error) {
	return e.Host.Execute(command)
}

// NewHostFromRemote creates a Host instance from a components.RemoteHost
func NewHostFromDocker(host *components.RemoteHostDocker, containerName string) *Host {
	return &Host{
		Executor: &dockerExecutor{
			remote:        host,
			containerName: containerName,
		},
		OSFamily: componentos.LinuxFamily,
	}
}

type dockerExecutor struct {
	remote        *components.RemoteHostDocker
	containerName string
}

// Execute runs commands to satisfy the Executor interface
func (e *dockerExecutor) Execute(command string) (output string, err error) {
	return e.remote.Client.ExecuteCommandWithErr(
		e.containerName,
		// Wrap in a shell call to achieve similar behavior to remote hosts
		[]string{"sh", "-c", command}...,
	)
}
