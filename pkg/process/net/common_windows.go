// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package net

import (
	"fmt"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	connectionsURL = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/connections"
	registerURL    = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/register"
	statsURL       = "http://localhost:3333/debug/stats"
	netType        = "tcp"

	// procStatsURL is not used in windows, the value is added to avoid compilation error in windows
	procStatsURL = "http://localhost:3333/" + string(sysconfig.ProcessModule) + "stats"
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}
	return nil
}
