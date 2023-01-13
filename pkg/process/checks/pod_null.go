// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet || !orchestrator
// +build !kubelet !orchestrator

package checks

import (
	"fmt"
)

// Pod is a singleton PodCheck.
var Pod = &PodCheck{}

// PodCheck is a check that returns container metadata and stats.
type PodCheck struct {
}

// Init initializes a PodCheck instance.
func (c *PodCheck) Init(_ *SysProbeConfig, hostInfo *HostInfo) error {
	return nil
}

func (c *PodCheck) IsEnabled() bool {
	return false
}

func (c *PodCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ProcessCheck.
func (c *PodCheck) Name() string { return "pod" }

// Realtime indicates if this check only runs in real-time mode.
func (c *PodCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *PodCheck) ShouldSaveLastRun() bool { return true }

// Run runs the PodCheck to collect a list of running pods
func (c *PodCheck) Run(_ func() int32, _ *RunOptions) (RunResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// Cleanup frees any resource held by the PodCheck before the agent exits
func (c *PodCheck) Cleanup() {}
