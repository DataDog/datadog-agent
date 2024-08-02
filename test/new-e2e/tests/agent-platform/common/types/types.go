// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides types for uses in agent-platform tests
package types

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
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

// MustExecute executes a command and requires no error.
func (h *Host) MustExecute(t *testing.T, command string) string {
	stdout, err := h.Execute(command)
	require.NoError(t, err)
	return stdout
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
