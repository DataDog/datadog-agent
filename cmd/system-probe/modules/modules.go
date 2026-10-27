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
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

var all []*module.Factory

var moduleOrder = []types.ModuleName{
	types.EBPFModule,
	types.NetworkTracerModule,
	types.TCPQueueLengthTracerModule,
	types.OOMKillProbeModule,
	types.EventMonitorModule, // there is a dependency from EventMonitor -> NetworkTracer, so EventMonitor has to follow NetworkTracer
	types.ProcessModule,
	types.DynamicInstrumentationModule, // dynamic instrumentation needs to be after EventMonitor
	types.LanguageDetectionModule,
	types.ComplianceModule,
	types.PingModule,
	types.TracerouteModule,
	types.DiscoveryModule,
	types.GPUMonitoringModule, // GPU monitoring needs to be initialized after EventMonitor, so that we have the event consumer ready
	types.SoftwareInventoryModule,
	types.NotableEventsModule,
	types.PrivilegedLogsModule,
	types.InjectorModule,
	types.NoisyNeighborModule,
	types.LogonDurationModule,
}

//nolint:unused // may be unused with certain build tag combinations
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
