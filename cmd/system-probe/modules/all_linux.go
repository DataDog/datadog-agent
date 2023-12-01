// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
)

// All System Probe modules should register their factories here
var All = []module.Factory{
	EBPFProbe,
	NetworkTracer,
	TCPQueueLength,
	OOMKillProbe,
	EventMonitor,
	Process,
	DynamicInstrumentation,
	LanguageDetectionModule,
	ComplianceModule,
}

func inactivityEventLog(duration time.Duration) {

}
