// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

const (
	// Namespace is the top-level configuration key that all system-probe settings are nested underneath
	Namespace = "system_probe_config"
)

// system-probe module names
const (
	NetworkTracerModule          types.ModuleName = "network_tracer"
	OOMKillProbeModule           types.ModuleName = "oom_kill_probe"
	TCPQueueLengthTracerModule   types.ModuleName = "tcp_queue_length_tracer"
	ProcessModule                types.ModuleName = "process"
	EventMonitorModule           types.ModuleName = "event_monitor"
	DynamicInstrumentationModule types.ModuleName = "dynamic_instrumentation"
	EBPFModule                   types.ModuleName = "ebpf"
	LanguageDetectionModule      types.ModuleName = "language_detection"
	WindowsCrashDetectModule     types.ModuleName = "windows_crash_detection"
	ComplianceModule             types.ModuleName = "compliance"
	PingModule                   types.ModuleName = "ping"
	TracerouteModule             types.ModuleName = "traceroute"
	DiscoveryModule              types.ModuleName = "discovery"
	GPUMonitoringModule          types.ModuleName = "gpu"
	SoftwareInventoryModule      types.ModuleName = "software_inventory"
	PrivilegedLogsModule         types.ModuleName = "privileged_logs"
	InjectorModule               types.ModuleName = "injector"
	NoisyNeighborModule          types.ModuleName = "noisy_neighbor"
)

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string, fleetPoliciesDirPath string) (*types.Config, error) {
	return newSysprobeConfig(configPath, fleetPoliciesDirPath)
}

func newSysprobeConfig(configPath string, fleetPoliciesDirPath string) (*types.Config, error) {
	cfg := pkgconfigsetup.GlobalSystemProbeConfigBuilder()

	cfg.SetConfigName("system-probe")
	// set the paths where a config file is expected
	if len(configPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first, so it's first in line
		cfg.AddConfigPath(configPath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(configPath, ".yaml") {
			cfg.SetConfigFile(configPath)
		}
	} else {
		// only add default if a custom configPath was not supplied
		cfg.AddConfigPath(defaultConfigDir)
	}
	// load the configuration
	ddcfg := pkgconfigsetup.Datadog()
	err := pkgconfigsetup.LoadSystemProbe(cfg, ddcfg.GetEnvVars())
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			// special-case permission-denied with a clearer error message
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf(`cannot access the system-probe config file (%w); try running the command in an Administrator shell"`, err)
			}
			return nil, fmt.Errorf("cannot access the system-probe config file (%w); try running the command under the same user as the Datadog Agent", err)
		}

		if !errors.Is(err, pkgconfigmodel.ErrConfigFileNotFound) && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("unable to load system-probe config file: %w", err)
		}
	}

	// if fleetPoliciesDirPath was provided in the command line, copy it to the config
	if fleetPoliciesDirPath != "" {
		cfg.Set("fleet_policies_dir", fleetPoliciesDirPath, pkgconfigmodel.SourceAgentRuntime)
	}
	// apply remote fleet policy to the config
	err = applyFleetPolicy(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load fleet policy: %w", err)
	}

	return load()
}

func load() (*types.Config, error) {
	cfg := pkgconfigsetup.GlobalSystemProbeConfigBuilder()
	coreCfg := pkgconfigsetup.Datadog()

	Adjust(cfg)

	c := &types.Config{
		Enabled:             cfg.GetBool(spNS("enabled")),
		EnabledModules:      make(map[types.ModuleName]struct{}),
		ExternalSystemProbe: cfg.GetBool(spNS("external")),

		SocketAddress:      cfg.GetString(spNS("sysprobe_socket")),
		MaxConnsPerMessage: cfg.GetInt(spNS("max_conns_per_message")),

		LogFile:          cfg.GetString("log_file"),
		LogLevel:         cfg.GetString("log_level"),
		DebugPort:        cfg.GetInt(spNS("debug_port")),
		HealthPort:       cfg.GetInt(spNS("health_port")),
		TelemetryEnabled: cfg.GetBool(spNS("telemetry_enabled")),
	}

	npmEnabled := cfg.GetBool(netNS("enabled"))
	usmEnabled := cfg.GetBool(smNS("enabled"))
	ccmEnabled := cfg.GetBool(ccmNS("enabled"))
	csmEnabled := cfg.GetBool(secNS("enabled"))
	gpuEnabled := cfg.GetBool(gpuNS("enabled"))
	diEnabled := cfg.GetBool(diNS("enabled"))
	swEnabled := coreCfg.GetBool(swNS("enabled"))

	if npmEnabled || usmEnabled || ccmEnabled || (csmEnabled && cfg.GetBool(secNS("network_monitoring.enabled"))) {
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	}
	if cfg.GetBool(spNS("enable_tcp_queue_length")) {
		c.EnabledModules[TCPQueueLengthTracerModule] = struct{}{}
	}
	if cfg.GetBool(spNS("enable_oom_kill")) {
		c.EnabledModules[OOMKillProbeModule] = struct{}{}
	}
	if csmEnabled ||
		cfg.GetBool(secNS("fim_enabled")) ||
		(usmEnabled && cfg.GetBool(smNS("enable_event_stream"))) ||
		(c.ModuleIsEnabled(NetworkTracerModule) && cfg.GetBool(evNS("network_process.enabled"))) ||
		gpuEnabled ||
		diEnabled {
		c.EnabledModules[EventMonitorModule] = struct{}{}
	}
	complianceEnabled := coreCfg.GetBool(compNS("enabled"))
	complianceRunInSystemProbe := coreCfg.GetBool(compNS("run_in_system_probe"))
	complianceDBBenchmarksEnabled := cfg.GetBool(compNS("database_benchmarks.enabled"))
	complianceLegacyCWSEnabled := cfg.GetBool(secNS("enabled")) && cfg.GetBool(secNS("compliance_module.enabled"))

	// Enable compliance module if:
	// 1. Full compliance is enabled AND should run in system-probe, OR
	// 2. Only DB benchmarks handler is needed (regardless of run_in_system_probe), OR
	// 3. Legacy CWS config enables compliance module
	shouldEnableComplianceModule := (complianceEnabled && complianceRunInSystemProbe) || complianceDBBenchmarksEnabled || complianceLegacyCWSEnabled

	if shouldEnableComplianceModule {
		c.EnabledModules[ComplianceModule] = struct{}{}
	}
	if cfg.GetBool(spNS("process_config.enabled")) {
		c.EnabledModules[ProcessModule] = struct{}{}
	}
	if diEnabled {
		c.EnabledModules[DynamicInstrumentationModule] = struct{}{}
	}
	if cfg.GetBool(NSkey("ebpf_check", "enabled")) {
		c.EnabledModules[EBPFModule] = struct{}{}
	}
	if cfg.GetBool("system_probe_config.language_detection.enabled") {
		c.EnabledModules[LanguageDetectionModule] = struct{}{}
	}
	if cfg.GetBool(pngNS("enabled")) {
		c.EnabledModules[PingModule] = struct{}{}
	}
	if cfg.GetBool(tracerouteNS("enabled")) {
		c.EnabledModules[TracerouteModule] = struct{}{}
	}
	if cfg.GetBool(discoveryNS("enabled")) {
		c.EnabledModules[DiscoveryModule] = struct{}{}
	}
	if gpuEnabled {
		c.EnabledModules[GPUMonitoringModule] = struct{}{}
	}
	if cfg.GetBool(privilegedLogsNS("enabled")) {
		c.EnabledModules[PrivilegedLogsModule] = struct{}{}
	}
	if cfg.GetBool(NSkey("noisy_neighbor", "enabled")) {
		c.EnabledModules[NoisyNeighborModule] = struct{}{}
	}

	if cfg.GetBool(wcdNS("enabled")) {
		c.EnabledModules[WindowsCrashDetectModule] = struct{}{}
	}

	if runtime.GOOS == "windows" {
		if c.ModuleIsEnabled(NetworkTracerModule) || c.ModuleIsEnabled(EventMonitorModule) {
			// enable the windows crash detection module if the network tracer
			// module is enabled, to allow the core agent to detect our own crash
			c.EnabledModules[WindowsCrashDetectModule] = struct{}{}
		}
		if swEnabled {
			c.EnabledModules[SoftwareInventoryModule] = struct{}{}
		}

		// injector telemetry is enabled by default, disable only if explicitly configured by the user
		injectorDefaultEnabled := false
		if !cfg.IsConfigured("injector.enable_telemetry") {
			injectorDefaultEnabled = true
		} else if cfg.GetBool("injector.enable_telemetry") {
			c.EnabledModules[InjectorModule] = struct{}{}
		}

		// This check must be last for any default modules on Windows,
		// Only add default modules if other explicit modules have been enabled
		// because the count of enabled modules will implicitly enable system probe.
		if len(c.EnabledModules) > 0 && injectorDefaultEnabled {
			c.EnabledModules[InjectorModule] = struct{}{}
		}
	}

	// Enable discovery by default on Linux if system-probe has any modules
	// enabled, unless the user has explicitly configured the discovery.enabled
	// config key.
	//
	// Note that besides the support in system-probe itself (currently only
	// implemented on Linux), the WorkloadMeta-based process collector in the
	// core agent needs to be supported on the platform for discovery to work
	// correctly.
	if runtime.GOOS == "linux" &&
		len(c.EnabledModules) > 0 &&
		!c.ModuleIsEnabled(DiscoveryModule) &&
		applyDefault(cfg, discoveryNS("enabled"), true) {
		c.EnabledModules[DiscoveryModule] = struct{}{}
	}

	c.Enabled = len(c.EnabledModules) > 0
	// only allowed raw config adjustments here, otherwise use Adjust function
	cfg.Set(spNS("enabled"), c.Enabled, pkgconfigmodel.SourceAgentRuntime)

	return c, nil
}

func applyFleetPolicy(cfg pkgconfigmodel.Config) error {
	// Apply overrides for local config options (e.g. fleet_policies_dir)
	pkgconfigsetup.FleetConfigOverride(cfg)

	// Load the remote configuration
	fleetPoliciesDirPath := cfg.GetString("fleet_policies_dir")
	if fleetPoliciesDirPath != "" {
		err := cfg.MergeFleetPolicy(path.Join(fleetPoliciesDirPath, "system-probe.yaml"))
		if err != nil {
			return fmt.Errorf("failed to merge fleet policy: %w", err)
		}
	}

	return nil
}
