// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workerclient runs the e2ectl-worker binary (the pulumi-executor)
// for cloud provisioning. The versioned job stays independent of provider
// fields: each scenario revalidates a normalized payload with its shared schema.
package workerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
)

// ProtocolVersion guards the normalized typed-config and fixture handoff.
const ProtocolVersion = 1

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

// Job is the versioned JSON contract with the Pulumi executor. New scenarios
// register a shared schema and run function, not new fields on this envelope.
type Job struct {
	ProtocolVersion int              `json:"protocol_version"`
	Action          string           `json:"action"`
	Base            string           `json:"base"`
	Fixtures        *fixtures.Config `json:"fixtures"`
	// Params is normalized driver-owned YAML with defaults already materialized.
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
	if job.ProtocolVersion != ProtocolVersion || job.Fixtures == nil {
		return fmt.Errorf("invalid executor job: protocol version %d and explicit fixture settings are required", ProtocolVersion)
	}
	worker, err := binaryPath()
	if err != nil {
		return err
	}
	// Old executors may otherwise ignore new fields and provision with different
	// defaults. Negotiate before starting any infrastructure operation.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	version, err := exec.CommandContext(ctx, worker, "--protocol-version").Output()
	if err != nil || strings.TrimSpace(string(version)) != strconv.Itoa(ProtocolVersion) {
		return fmt.Errorf("executor %s does not support protocol %d; rebuild both e2ectl and e2ectl-worker", worker, ProtocolVersion)
	}
	jobPath := filepath.Join(dir, "job.json")
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	// Normalized parameters may contain explicit runtime secrets. Replace even
	// an older permissive job file with a new private file.
	f, err := os.CreateTemp(dir, ".job-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), jobPath); err != nil {
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
