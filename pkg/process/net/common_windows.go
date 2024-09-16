// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"fmt"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	connectionsURL       = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/connections"
	networkIDURL         = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/network_id"
	registerURL          = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/register"
	languageDetectionURL = "http://localhost:3333/" + string(sysconfig.LanguageDetectionModule) + "/detect"
	statsURL             = "http://localhost:3333/debug/stats"
	pprofURL             = "http://localhost:3333/debug/pprof"
	tracerouteURL        = "http://localhost:3333/" + string(sysconfig.TracerouteModule) + "/traceroute/"
	netType              = "tcp"

	// discovery* is not used on Windows, the value is added to avoid a compilation error
	discoveryServicesURL = "http://localhost:3333/" + string(sysconfig.DiscoveryModule) + "/services"
	// procStatsURL is not used in windows, the value is added to avoid compilation error in windows
	procStatsURL = "http://localhost:3333/" + string(sysconfig.ProcessModule) + "stats"
	// pingURL is not used in windows, the value is added to avoid compilation error in windows
	pingURL = "http://localhost:3333/" + string(sysconfig.PingModule) + "/ping/"

	// SystemProbePipeName is the production named pipe for system probe
	SystemProbePipeName = `\\.\pipe\dd_system_probe`
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path is empty")
	}
	return nil
}
