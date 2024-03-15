// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

//nolint:revive // TODO(PROC) Fix revive linter
package net

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// CheckPath is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func CheckPath(path string) error {
	return ErrNotImplemented
}

// GetRemoteSystemProbeUtil is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func GetRemoteSystemProbeUtil(path string) (*RemoteSysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, ErrNotImplemented
}

// GetConnections is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, ErrNotImplemented
}

// GetStats is not supported
func (r *RemoteSysProbeUtil) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// GetProcStats is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, ErrNotImplemented
}

// Register is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) Register(clientID string) error {
	return ErrNotImplemented
}

// DetectLanguage is not supported
func (r *RemoteSysProbeUtil) DetectLanguage([]int32) ([]languagemodels.Language, error) {
	return nil, ErrNotImplemented
}
