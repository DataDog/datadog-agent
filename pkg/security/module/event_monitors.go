// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	netevents "github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// InitEventMonitors initialize event monitors
func InitEventMonitors(module *Module) {
	initProcessMonitor(module)
	initNetworkProcessMonitor(module)
}

func initProcessMonitor(module *Module) {
	pm := NewProcessMonitoring(module)
	module.probe.AddEventHandler(model.ForkEventType, pm)
	module.probe.AddEventHandler(model.ExecEventType, pm)
	module.probe.AddEventHandler(model.ExitEventType, pm)
}

func initNetworkProcessMonitor(module *Module) {
	m := netevents.Handler()
	module.probe.AddEventHandler(model.ForkEventType, m)
	module.probe.AddEventHandler(model.ExecEventType, m)
	module.probe.AddEventHandler(model.ExitEventType, m)
}
