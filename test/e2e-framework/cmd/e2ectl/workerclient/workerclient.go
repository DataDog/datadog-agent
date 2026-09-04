// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package workerclient runs the e2ectl-worker binary (the pulumi-executor)
// for cloud provisioning. The job is generic forever: {action, base, params}.
// Params is the raw driver-owned config section (YAML) that the registered
// scenario strict-decodes — the same payload, the same rule, at every layer.
package workerclient

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Base IDs — the one shared place where the two binaries agree (T4).
const (
	BaseKind    = "kind"
	BaseEC2Host = "ec2-host"
)

// Actions.
const (
	ActionProvision = "provision"
	ActionDestroy   = "destroy"
)

// Job is the JSON contract with the pulumi-executor. Its shape never changes:
// new scenarios register {base → params decoder + run function} on the worker
// side; the core never grows per-provider fields again.
type Job struct {
	Action string `json:"action"`
	Base   string `json:"base"`
	// Params is the raw driver-owned config section (YAML text).
	Params string `json:"params,omitempty"`
	// StackName is the Pulumi stack for this environment: executor-wide
	// bookkeeping, computed deterministically by the driver.
	StackName string `json:"stack_name,omitempty"`
	// EnvDir is the envstore entry directory (snapshot + outputs live there).
	EnvDir string `json:"env_dir"`
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
		return fmt.Errorf("e2ectl-worker %s %s: %w", job.Action, job.Base, err)
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
