// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"errors"
	"os"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/process"
)

// GetSelfContainerID returns the container ID of the current process by reading cgroup.
func GetSelfContainerID() (string, error) {
	pid := libpf.PID(os.Getpid())
	id := process.New(pid, pid).GetProcessMeta(process.MetaConfig{}).ContainerID.String()
	if id == "" {
		return "", errors.New("not running in a container")
	}
	return id, nil
}
