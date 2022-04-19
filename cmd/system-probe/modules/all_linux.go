// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package modules

import (
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
)

// All System Probe modules should register their factories here
var All = []module.Factory{
	// Compiler must always be first module in this list so that if runtime compilation is required, it occurs
	// prior to any other modules starting.
	Compiler,
	NetworkTracer,
	TCPQueueLength,
	OOMKillProbe,
	SecurityRuntime,
	Process,
}

func inactivityEventLog(duration time.Duration) {

}
