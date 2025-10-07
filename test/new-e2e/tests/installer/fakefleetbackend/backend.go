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

// type remoteAPIRequest struct {
// 	ID            string          `json:"id"`
// 	Package       string          `json:"package_name"`
// 	TraceID       string          `json:"trace_id"`
// 	ParentSpanID  string          `json:"parent_span_id"`
// 	ExpectedState expectedState   `json:"expected_state"`
// 	Method        string          `json:"method"`
// 	Params        json.RawMessage `json:"params"`
// }

// type expectedState struct {
// 	InstallerVersion string `json:"installer_version"`
// 	Stable           string `json:"stable"`
// 	Experiment       string `json:"experiment"`
// 	StableConfig     string `json:"stable_config"`
// 	ExperimentConfig string `json:"experiment_config"`
// }

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

// ConfigureAgent configures the agent with the given operations.
func (b *Backend) ConfigureAgent(_ ConfigOperations) error {
	status, err := b.RemoteConfigStatus()
	return fmt.Errorf("TEST ERROR: %s\n\n%v", status, err)
}

// RemoteConfigStatus returns the status of the remote config.
func (b *Backend) RemoteConfigStatus() (string, error) {
	return b.runDaemonCommand("rc-status")
}

func (b *Backend) runDaemonCommand(command string, args ...string) (string, error) {
	return b.remote.Execute(fmt.Sprintf("sudo DD_BUNDLED_AGENT=installer datadog-agent daemon %s %s", command, strings.Join(args, " ")))
}
