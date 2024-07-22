// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package net

import (
	"fmt"

	model "github.com/DataDog/agent-payload/v5/process"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	statsURL             = "http://unix/debug/stats"
	registerURL          = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/register"
	tracerouteURL        = "http://unix/" + string(sysconfig.TracerouteModule) + "/traceroute/"
	languageDetectionURL = "http://unix/" + string(sysconfig.LanguageDetectionModule) + "/detect"
	pprofURL             = "http://unix/debug/pprof"

	// pingURL is not used in windows, the value is added to avoid compilation error in windows
	pingURL = "http://unix/" + string(sysconfig.PingModule) + "/ping/"

	netType = "unix"
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path is empty")
	}
	return nil
}

// GetConnections is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	return nil, ErrNotImplemented
}

// GetProcStats is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, ErrNotImplemented
}
