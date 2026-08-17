// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	ddProcmgrServiceName = "dd-procmgr-service"
	// ddProcmgrRestartTimeout bounds SCM restart after processes.d changes.
	ddProcmgrRestartTimeout = 120 * time.Second
)

// RestartProcmgrService restarts dd-procmgr-service so it re-reads processes.d from disk.
// No-op when the service is already stopped (e.g. MSI prerm runs after StopDDServices).
func RestartProcmgrService() {
	if paths.DatadogProgramFilesDir == "" {
		log.Warnf("procmgr: DatadogProgramFilesDir is empty; cannot restart %s", ddProcmgrServiceName)
		return
	}
	running, err := winutil.IsServiceRunning(ddProcmgrServiceName)
	if err != nil {
		log.Warnf("procmgr: could not query %s state before restart: %v", ddProcmgrServiceName, err)
		return
	}
	if !running {
		log.Debugf("procmgr: skip restart; %s is not running", ddProcmgrServiceName)
		return
	}
	if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, ddProcmgrRestartTimeout); err != nil {
		log.Warnf("procmgr: failed to restart %s after processes.d change: %v", ddProcmgrServiceName, err)
	}
}
