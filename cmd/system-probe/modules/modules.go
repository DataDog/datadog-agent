// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package modules is all the module definitions for system-probe
package modules

import (
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

var all []*module.Factory

var moduleOrder = []types.ModuleName{
	config.EBPFModule,
	config.NetworkTracerModule,
	config.TCPQueueLengthTracerModule,
	config.OOMKillProbeModule,
	config.EventMonitorModule, // there is a dependency from EventMonitor -> NetworkTracer, so EventMonitor has to follow NetworkTracer
	config.ProcessModule,
	config.DynamicInstrumentationModule,
	config.LanguageDetectionModule,
	config.ComplianceModule,
	config.PingModule,
	config.TracerouteModule,
	config.DiscoveryModule,
	config.GPUMonitoringModule, // GPU monitoring needs to be initialized after EventMonitor, so that we have the event consumer ready
}

// nolint: deadcode, unused // may be unused with certain build tag combinations
func registerModule(mod *module.Factory) {
	if mod.Name == "" {
		return
	}
	all = append(all, mod)
}

// All is the list of supported modules in the order specified by `moduleOrder`
var All = sync.OnceValue(func() []*module.Factory {
	slices.SortStableFunc(all, func(a, b *module.Factory) int {
		return slices.Index(moduleOrder, a.Name) - slices.Index(moduleOrder, b.Name)
	})
	return all
})
