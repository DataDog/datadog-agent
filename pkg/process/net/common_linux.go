// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package net

import (
	"fmt"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"os"
)

const (
	connectionsEndpoint = "connections"
	procStatsEndpoint   = "stats"
	registerEndpoint    = "register"

	urlPrefix = "http://unix"
	statsURL  = "http://unix/debug/stats"
	netType   = "unix"
)

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the socket exists
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}

	if _, err := os.Stat(globalSocketPath); err != nil {
		return fmt.Errorf("socket path does not exist: %v", err)
	}
	return nil
}

func GetConnectionsURL() string {
	return fmt.Sprintf("%s/%s/%s", urlPrefix, string(sysconfig.NetworkTracerModule), connectionsEndpoint)
}

func GetProcStatsURL() string {
	return fmt.Sprintf("%s/%s/%s", urlPrefix, string(sysconfig.ProcessModule), procStatsEndpoint)
}

func GetRegisterURL() string {
	return fmt.Sprintf("%s/%s/%s", urlPrefix, string(sysconfig.NetworkTracerModule), registerEndpoint)
}
