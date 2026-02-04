// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type SystemProbeModuleComponent interface {
	Name() types.ModuleName
	ConfigNamespaces() []string
	Create() (SystemProbeModule, error)
	NeedsEBPF() bool
	OptionalEBPF() bool
}

// ModuleOrder is the desired creation order for system-probe modules
var ModuleOrder = []types.ModuleName{
	config.EBPFModule,
	config.NetworkTracerModule,
	config.TCPQueueLengthTracerModule,
	config.OOMKillProbeModule,
	config.EventMonitorModule, // there is a dependency from EventMonitor -> NetworkTracer, so EventMonitor has to follow NetworkTracer
	config.ProcessModule,
	config.DynamicInstrumentationModule, // dynamic instrumentation needs to be after EventMonitor
	config.LanguageDetectionModule,
	config.ComplianceModule,
	config.PingModule,
	config.TracerouteModule,
	config.DiscoveryModule,
	config.GPUMonitoringModule, // GPU monitoring needs to be initialized after EventMonitor, so that we have the event consumer ready
	config.SoftwareInventoryModule,
	config.PrivilegedLogsModule,
	config.WindowsCrashDetectModule,
}
