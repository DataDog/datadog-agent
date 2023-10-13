// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package modules

import (
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"
)

// All System Probe modules should register their factories here
var All = []module.Factory{
	NetworkTracer,
	EventMonitor,
	WinCrashProbe,
}

func inactivityEventLog(duration time.Duration) {
	winutil.LogEventViewer(config.ServiceName, messagestrings.MSG_SYSPROBE_RESTART_INACTIVITY, duration.String())
}
