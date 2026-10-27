// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the different types used by the system-probe config.
//
// This types are extracted to their own module so other can link against it without compiling with the entire
// system-probe code base.
package types

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

// system-probe module names
const (
	NetworkTracerModule          ModuleName = "network_tracer"
	OOMKillProbeModule           ModuleName = "oom_kill_probe"
	TCPQueueLengthTracerModule   ModuleName = "tcp_queue_length_tracer"
	ProcessModule                ModuleName = "process"
	EventMonitorModule           ModuleName = "event_monitor"
	DynamicInstrumentationModule ModuleName = "dynamic_instrumentation"
	EBPFModule                   ModuleName = "ebpf"
	LanguageDetectionModule      ModuleName = "language_detection"
	WindowsCrashDetectModule     ModuleName = "windows_crash_detection"
	ComplianceModule             ModuleName = "compliance"
	PingModule                   ModuleName = "ping"
	TracerouteModule             ModuleName = "traceroute"
	DiscoveryModule              ModuleName = "discovery"
	GPUMonitoringModule          ModuleName = "gpu"
	SoftwareInventoryModule      ModuleName = "software_inventory"
	NotableEventsModule          ModuleName = "notable_events"
	PrivilegedLogsModule         ModuleName = "privileged_logs"
	InjectorModule               ModuleName = "injector"
	NoisyNeighborModule          ModuleName = "noisy_neighbor"
	LogonDurationModule          ModuleName = "logon_duration"
)

// Config represents the configuration options for the system-probe
type Config struct {
	Enabled        bool
	EnabledModules map[ModuleName]struct{}

	// When the system-probe is enabled in a separate container, we need a way to also disable the system-probe
	// packaged in the main agent container (without disabling network collection on the process-agent).
	ExternalSystemProbe bool

	SocketAddress      string
	MaxConnsPerMessage int

	LogFile          string
	LogLevel         string
	DebugPort        int
	HealthPort       int
	TelemetryEnabled bool
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}
