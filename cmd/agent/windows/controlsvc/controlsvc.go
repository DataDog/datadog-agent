// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package controlsvc contains shared code for controlling the Windows agent service.
package controlsvc

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// StartService starts the agent service via the Service Control Manager
func StartService() error {
	return winutil.StartService(config.ServiceName)
}

// RestartService restarts the agent service by calling StopService and StartService
func RestartService() error {
	return winutil.RestartService(config.ServiceName)
}

// StopService stops the agent service via the Service Control Manager
func StopService() error {
	return winutil.StopService(config.ServiceName)
}
