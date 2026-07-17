// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package slurm resolves Slurm job identity for a process from /proc/<pid>/environ. Slurm sets
// SLURM_JOB_ID (and related SLURM_JOB_* vars) unconditionally at task launch, so reading them
// back needs only the SYS_PTRACE capability -- no slurm binaries, no munge, and no hostPID.
package slurm

import (
	"fmt"
	"os"
	"strings"
	"sync"

	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
)

// SlurmInfo is the Slurm job identity resolved for a PID.
type SlurmInfo struct {
	// JobID is the numeric Slurm job ID, from SLURM_JOB_ID. Empty if unresolved.
	JobID string
	// JobName is the Slurm job name, from SLURM_JOB_NAME. Only set when JobID is set.
	JobName string
	// Partition is the Slurm partition, from SLURM_JOB_PARTITION. Only set when JobID is set.
	Partition string
}

// Provider resolves Slurm job identity for a PID.
type Provider interface {
	// GetSlurmInfo returns the Slurm job identity for pid. A zero-value SlurmInfo with a nil
	// error means the process is not a Slurm job (expected, not an error). A non-nil error
	// means the read was denied (e.g. the agent is missing the SYS_PTRACE capability needed to
	// read another process's /proc/<pid>/environ) and callers should treat it as a
	// misconfiguration signal rather than a normal "not a Slurm job" miss.
	GetSlurmInfo(pid int32) (SlurmInfo, error)
}

var (
	initOnce sync.Once
	shared   Provider
)

// InitSharedProvider initializes and returns the shared Provider singleton. Safe to call
// multiple times; only the first call constructs the provider.
func InitSharedProvider() Provider {
	initOnce.Do(func() {
		shared = &procProvider{}
	})
	return shared
}

// GetSharedProvider returns the shared Provider singleton, or nil if InitSharedProvider was
// never called.
func GetSharedProvider() Provider {
	return shared
}

type procProvider struct{}

const (
	envJobIDPrefix     = "SLURM_JOB_ID="
	envJobNamePrefix   = "SLURM_JOB_NAME="
	envPartitionPrefix = "SLURM_JOB_PARTITION="
)

// GetSlurmInfo implements Provider.
func (*procProvider) GetSlurmInfo(pid int32) (SlurmInfo, error) {
	var info SlurmInfo

	// maxEnvVars=0: EnvVars always collects every priority-prefixed var regardless of this cap
	// (the cap only bounds the second pass over non-matching vars), so passing 0 returns just
	// the SLURM_-prefixed entries instead of materializing the whole process environment.
	envs, _, envErr := secutils.EnvVars([]string{"SLURM_"}, uint32(pid), 0)
	if envErr == nil {
		for _, e := range envs {
			switch {
			case strings.HasPrefix(e, envJobIDPrefix):
				info.JobID = strings.TrimPrefix(e, envJobIDPrefix)
			case strings.HasPrefix(e, envJobNamePrefix):
				info.JobName = strings.TrimPrefix(e, envJobNamePrefix)
			case strings.HasPrefix(e, envPartitionPrefix):
				info.Partition = strings.TrimPrefix(e, envPartitionPrefix)
			}
		}
	}

	if info.JobID != "" {
		return info, nil
	}

	// No job ID resolved. Distinguish "not a Slurm job" (expected, silent) from
	// "misconfigured, can't read /proc" (a real problem callers should count) — the latter
	// requires SYS_PTRACE in the agent's securityContext to read another process's
	// /proc/<pid>/environ.
	if os.IsPermission(envErr) {
		return SlurmInfo{}, fmt.Errorf("permission denied resolving slurm info for pid %d: %w", pid, envErr)
	}

	return SlurmInfo{}, nil
}
