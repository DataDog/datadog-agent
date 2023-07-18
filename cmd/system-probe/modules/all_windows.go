// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package modules

import (
	"time"

	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// All System Probe modules should register their factories here
var All = []module.Factory{
	NetworkTracer,
	EventMonitor,
}

const (
	msgSysprobeRestartInactivity = 0x8000000f
)

func inactivityEventLog(duration time.Duration) {
	elog, err := eventlog.Open(config.ServiceName)
	if err != nil {
		return
	}
	defer elog.Close()
	_ = elog.Warning(msgSysprobeRestartInactivity, duration.String())
}
