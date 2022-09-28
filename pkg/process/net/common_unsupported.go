// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows
// +build !linux,!windows

package net

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// SetSystemProbePath is not supported
func SetSystemProbePath(_ string) {
	// no-op
}

// CheckPath is not supported
func CheckPath() error {
	return ebpf.ErrNotImplemented
}

// GetRemoteSystemProbeUtil is not supported
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, ebpf.ErrNotImplemented
}

// GetConnections is not supported
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetStats is not supported
func (r *RemoteSysProbeUtil) GetStats() (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetProcStats is not supported
func (r *RemoteSysProbeUtil) GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, ebpf.ErrNotImplemented
}

// Register is not supported
func (r *RemoteSysProbeUtil) Register(clientID string) error {
	return ebpf.ErrNotImplemented
}
