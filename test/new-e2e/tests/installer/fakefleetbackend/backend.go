// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fakefleetbackend contains a fake fleet backend for use in tests.
package fakefleetbackend

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// RemoteConfigState is the state of the remote config.
type RemoteConfigState struct {
	Packages []RemoteConfigStatePackage `json:"remote_config_state"`
}

// RemoteConfigStatePackage is the state of a package in the remote config.
type RemoteConfigStatePackage struct {
	Package                 string `json:"package"`
	StableVersion           string `json:"stable_version"`
	ExperimentVersion       string `json:"experiment_version"`
	StableConfigVersion     string `json:"stable_config_version"`
	ExperimentConfigVersion string `json:"experiment_config_version"`
}

// Backend is the fake fleet backend.
type Backend struct {
	t      func() *testing.T
	remote *components.RemoteHost
}

// New creates a new Backend.
func New(t func() *testing.T, remote *components.RemoteHost) *Backend {
	return &Backend{t: t, remote: remote}
}

// FileOperationType is the type of operation to perform on the config.
type FileOperationType string

const (
	// FileOperationPatch patches the config at the given path with the given JSON patch (RFC 6902).
	FileOperationPatch FileOperationType = "patch"
	// FileOperationMergePatch merges the config at the given path with the given JSON merge patch (RFC 7396).
	FileOperationMergePatch FileOperationType = "merge-patch"
	// FileOperationDelete deletes the config at the given path.
	FileOperationDelete FileOperationType = "delete"
)

// ConfigOperations is the list of operations to perform on the config.
type ConfigOperations struct {
	DeploymentID   string          `json:"deployment_id"`
	FileOperations []FileOperation `json:"file_operations"`
}

// FileOperation is the operation to perform on a config.
type FileOperation struct {
	FileOperationType FileOperationType `json:"file_op"`
	FilePath          string            `json:"file_path"`
	Patch             json.RawMessage   `json:"patch,omitempty"`
}

// StartConfigExperiment starts a config experiment for the given package.
func (b *Backend) StartConfigExperiment(operations ConfigOperations) error {
	b.t().Logf("Starting config experiment")
	rawOperations, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	output, err := b.runDaemonCommand("start-config-experiment", "datadog-agent", string(rawOperations))
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment started")
	return nil
}

// PromoteConfigExperiment promotes a config experiment for the given package.
func (b *Backend) PromoteConfigExperiment() error {
	b.t().Logf("Promoting config experiment")
	output, err := b.runDaemonCommand("promote-config-experiment", "datadog-agent")
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment promoted")
	return nil
}

// RemoteConfigStatusPackage returns the status of the remote config for a given package.
func (b *Backend) RemoteConfigStatusPackage(packageName string) (RemoteConfigStatePackage, error) {
	status, err := b.RemoteConfigStatus()
	if err != nil {
		return RemoteConfigStatePackage{}, err
	}
	for _, pkg := range status.Packages {
		if pkg.Package == packageName {
			return pkg, nil
		}
	}
	return RemoteConfigStatePackage{}, fmt.Errorf("package %s not found", packageName)
}

// RemoteConfigStatus returns the status of the remote config.
func (b *Backend) RemoteConfigStatus() (RemoteConfigState, error) {
	status, err := b.runDaemonCommand("rc-status")
	if err != nil {
		return RemoteConfigState{}, err
	}
	var remoteConfigState RemoteConfigState
	err = json.Unmarshal([]byte(status), &remoteConfigState)
	if err != nil {
		return RemoteConfigState{}, err
	}
	return remoteConfigState, nil
}

func (b *Backend) runDaemonCommand(command string, args ...string) (string, error) {
	return b.remote.Execute(fmt.Sprintf("sudo DD_BUNDLED_AGENT=installer datadog-agent daemon %s %s", command, strings.Join(args, " ")))
}
