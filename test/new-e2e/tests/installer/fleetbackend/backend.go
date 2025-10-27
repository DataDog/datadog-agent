// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fleetbackend contains a fake fleet backend for use in tests.
package fleetbackend

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/avast/retry-go/v4"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
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
	t    func() *testing.T
	host *environments.Host
}

// New creates a new Backend.
func New(t func() *testing.T, host *environments.Host) *Backend {
	return &Backend{t: t, host: host}
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
	output, err := b.runDaemonCommandWithRestart("start-config-experiment", "datadog-agent", string(rawOperations))
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment started")
	return nil
}

// PromoteConfigExperiment promotes a config experiment for the given package.
func (b *Backend) PromoteConfigExperiment() error {
	b.t().Logf("Promoting config experiment")
	output, err := b.runDaemonCommandWithRestart("promote-config-experiment", "datadog-agent")
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment promoted")
	return nil
}

// StopConfigExperiment stops a config experiment for the given package.
func (b *Backend) StopConfigExperiment() error {
	b.t().Logf("Stopping config experiment")
	output, err := b.runDaemonCommandWithRestart("stop-config-experiment", "datadog-agent")
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment stopped")
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

func (b *Backend) runDaemonCommandWithRestart(command string, args ...string) (string, error) {
	originalPID, err := b.getDaemonPID()
	if err != nil {
		return "", err
	}
	output, err := b.runDaemonCommand(command, args...)
	if err != nil {
		return "", err
	}
	err = retry.Do(func() error {
		newPID, err := b.getDaemonPID()
		if err != nil {
			return err
		}
		if newPID == originalPID {
			return fmt.Errorf("daemon PID %d is still running", newPID)
		}
		return nil
	}, retry.Attempts(5), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return "", fmt.Errorf("error waiting for daemon to restart: %w", err)
	}
	return output, nil
}

func (b *Backend) runDaemonCommand(command string, args ...string) (string, error) {
	var baseCommand string
	var sanitizeCharacter string
	switch b.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		sanitizeCharacter = `\"`
		baseCommand = "sudo DD_BUNDLED_AGENT=installer datadog-agent daemon"
	case e2eos.WindowsFamily:
		sanitizeCharacter = "\\`\""
		baseCommand = `& "C:\Program Files\Datadog\Datadog Agent\bin\datadog-installer.exe" daemon`
	default:
		return "", fmt.Errorf("unsupported OS family: %v", b.host.RemoteHost.OSFamily)
	}

	err := retry.Do(func() error {
		_, err := b.host.RemoteHost.Execute(fmt.Sprintf("%s rc-status", baseCommand))
		return err
	})
	if err != nil {
		return "", fmt.Errorf("error waiting for daemon to be ready: %w", err)
	}

	var sanitizedArgs []string
	for _, arg := range args {
		arg = `"` + strings.ReplaceAll(arg, `"`, sanitizeCharacter) + `"`
		sanitizedArgs = append(sanitizedArgs, arg)
	}
	return b.host.RemoteHost.Execute(fmt.Sprintf("%s %s %s", baseCommand, command, strings.Join(sanitizedArgs, " ")))
}

func (b *Backend) getDaemonPID() (int, error) {
	var pid string
	var err error
	switch b.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		pid, err = b.host.RemoteHost.Execute(`systemctl show -p MainPID datadog-agent-installer | cut -d= -f2`)
		pidExp, errExp := b.host.RemoteHost.Execute(`systemctl show -p MainPID datadog-agent-installer-exp | cut -d= -f2`)
		pid = strings.TrimSpace(pid)
		pidExp = strings.TrimSpace(pidExp)
		if err != nil || errExp != nil {
			return 0, fmt.Errorf("error getting daemon PID: %w, %w", err, errExp)
		}
		if pidExp != "0" {
			pid = pidExp
		}
	case e2eos.WindowsFamily:
		pid, err = b.host.RemoteHost.Execute(`(Get-CimInstance Win32_Service -Filter "Name='Datadog Installer'").ProcessId`)
		pid = strings.TrimSpace(pid)
	default:
		return 0, fmt.Errorf("unsupported OS family: %v", b.host.RemoteHost.OSFamily)
	}
	if err != nil {
		return 0, err
	}
	if pid == "0" {
		return 0, fmt.Errorf("daemon PID is 0")
	}
	return strconv.Atoi(pid)
}
