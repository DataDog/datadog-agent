// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

//nolint:revive // TODO(PROC) Fix revive linter
package net

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	discoverymodel "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	nppayload "github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}
var _ SysProbeUtilGetter = GetRemoteSystemProbeUtil

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// CheckPath is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func CheckPath(_ string) error {
	return ErrNotImplemented
}

// GetRemoteSystemProbeUtil is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func GetRemoteSystemProbeUtil(_ string) (SysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, ErrNotImplemented
}

// GetConnections is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetConnections(_ string) (*model.Connections, error) {
	return nil, ErrNotImplemented
}

// GetNetworkID is not supported
func (r *RemoteSysProbeUtil) GetNetworkID() (string, error) {
	return "", ErrNotImplemented
}

// GetStats is not supported
func (r *RemoteSysProbeUtil) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// GetProcStats is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetProcStats(_ []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, ErrNotImplemented
}

// Register is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) Register(_ string) error {
	return ErrNotImplemented
}

// DetectLanguage is not supported
func (r *RemoteSysProbeUtil) DetectLanguage([]int32) ([]languagemodels.Language, error) {
	return nil, ErrNotImplemented
}

// GetPprof is not supported
func (r *RemoteSysProbeUtil) GetPprof(_ string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// GetTelemetry is not supported
func (r *RemoteSysProbeUtil) GetTelemetry() ([]byte, error) { return nil, ErrNotImplemented }

// GetConnTrackCached is not supported
func (r *RemoteSysProbeUtil) GetConnTrackCached() ([]byte, error) { return nil, ErrNotImplemented }

// GetConnTrackHost is not supported
func (r *RemoteSysProbeUtil) GetConnTrackHost() ([]byte, error) { return nil, ErrNotImplemented }

// GetBTFLoaderInfo is not supported
func (r *RemoteSysProbeUtil) GetBTFLoaderInfo() ([]byte, error) { return nil, ErrNotImplemented }

func (r *RemoteSysProbeUtil) GetDiscoveryServices() (*discoverymodel.ServicesResponse, error) {
	return nil, ErrNotImplemented
}

func (r *RemoteSysProbeUtil) GetCheck(module sysconfigtypes.ModuleName) (interface{}, error) {
	return nil, ErrNotImplemented
}

func (r *RemoteSysProbeUtil) GetPing(clientID string, host string, count int, interval time.Duration, timeout time.Duration) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (r *RemoteSysProbeUtil) GetTraceroute(clientID string, host string, port uint16, protocol nppayload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error) {
	return nil, ErrNotImplemented
}
