// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package workerclient runs the e2ectl-worker binary for the Pulumi-linked or
// heavyweight jobs. The core CLI stays Pulumi-free; the worker pays that cost
// in its own process.
package workerclient

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Job is the JSON contract with e2ectl-worker (mirrors its job struct).
type Job struct {
	Action       string            `json:"action"`
	EnvDir       string            `json:"env_dir"`
	StackName    string            `json:"stack_name,omitempty"`
	OS           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	InstanceType string            `json:"instance_type,omitempty"`
	FakeIntake   bool              `json:"fakeintake,omitempty"`
	Version      string            `json:"version,omitempty"`
	Image        string            `json:"image,omitempty"`
	AgentConfig  string            `json:"agent_config,omitempty"`
	Integrations map[string]string `json:"integrations,omitempty"`
}

// Run writes the job to dir and executes the worker with it. The worker's
// stdout/stderr stream through.
func Run(dir string, job Job) error {
	jobPath := filepath.Join(dir, "job.json")
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jobPath, data, 0o644); err != nil {
		return err
	}

	worker, err := binaryPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(worker, jobPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("e2ectl-worker %s: %w", job.Action, err)
	}
	return nil
}

// binaryPath locates the e2ectl-worker binary: next to the running binary, or
// from $E2ECTL_WORKER.
func binaryPath() (string, error) {
	if p := os.Getenv("E2ECTL_WORKER"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, "e2ectl-worker"+ext())
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("e2ectl-worker binary not found next to %s (set $E2ECTL_WORKER or build both binaries)", exe)
}

func ext() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
