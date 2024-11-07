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

	"github.com/DataDog/viper"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
)

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string, fleetPoliciesDirPath string) (*types.Config, error) {
	return newSysprobeConfig(configPath, fleetPoliciesDirPath)
}

func newSysprobeConfig(configPath string, fleetPoliciesDirPath string) (*types.Config, error) {
	pkgconfigsetup.SystemProbe().SetConfigName("system-probe")
	// set the paths where a config file is expected
	if len(configPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first, so it's first in line
		pkgconfigsetup.SystemProbe().AddConfigPath(configPath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(configPath, ".yaml") {
			pkgconfigsetup.SystemProbe().SetConfigFile(configPath)
		}
	} else {
		// only add default if a custom configPath was not supplied
		pkgconfigsetup.SystemProbe().AddConfigPath(defaultConfigDir)
	}
	// load the configuration
	err := pkgconfigsetup.LoadCustom(pkgconfigsetup.SystemProbe(), pkgconfigsetup.Datadog().GetEnvVars())
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			// special-case permission-denied with a clearer error message
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf(`cannot access the system-probe config file (%w); try running the command in an Administrator shell"`, err)
			}
			return nil, fmt.Errorf("cannot access the system-probe config file (%w); try running the command under the same user as the Datadog Agent", err)
		}

		var e viper.ConfigFileNotFoundError
		if !errors.As(err, &e) && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("unable to load system-probe config file: %w", err)
		}
	}

	// Load the remote configuration
	if fleetPoliciesDirPath == "" {
		fleetPoliciesDirPath = pkgconfigsetup.SystemProbe().GetString("fleet_policies_dir")
	}
	if fleetPoliciesDirPath != "" {
		err := pkgconfigsetup.SystemProbe().MergeFleetPolicy(path.Join(fleetPoliciesDirPath, "system-probe.yaml"))
		if err != nil {
			return nil, err
		}
	}

	return load()
}

func load() (*types.Config, error) {
	cfg := pkgconfigsetup.SystemProbe()
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

		StatsdHost: pkgconfigsetup.GetBindHost(pkgconfigsetup.Datadog()),
		StatsdPort: cfg.GetInt("dogstatsd_port"),
	}

	npmEnabled := cfg.GetBool(netNS("enabled"))
	usmEnabled := cfg.GetBool(smNS("enabled"))
	ccmEnabled := cfg.GetBool(ccmNS("enabled"))
	csmEnabled := cfg.GetBool(secNS("enabled"))

	if npmEnabled || usmEnabled || ccmEnabled || (csmEnabled && cfg.GetBool(secNS("network_monitoring.enabled"))) {
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	}
	if cfg.GetBool(spNS("enable_tcp_queue_length")) {
		c.EnabledModules[TCPQueueLengthTracerModule] = struct{}{}
	}
	if cfg.GetBool(spNS("enable_oom_kill")) {
		c.EnabledModules[OOMKillProbeModule] = struct{}{}
	}
	if cfg.GetBool(secNS("enabled")) ||
		cfg.GetBool(secNS("fim_enabled")) ||
		cfg.GetBool(evNS("process.enabled")) ||
		(c.ModuleIsEnabled(NetworkTracerModule) && cfg.GetBool(evNS("network_process.enabled"))) {
		c.EnabledModules[EventMonitorModule] = struct{}{}
	}
	if cfg.GetBool(secNS("enabled")) && cfg.GetBool(secNS("compliance_module.enabled")) {
		c.EnabledModules[ComplianceModule] = struct{}{}
	}
	if cfg.GetBool(spNS("process_config.enabled")) {
		c.EnabledModules[ProcessModule] = struct{}{}
	}
	if cfg.GetBool(diNS("enabled")) {
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
	if cfg.GetBool(gpuNS("enabled")) {
		c.EnabledModules[GPUMonitoringModule] = struct{}{}
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
	}

	c.Enabled = len(c.EnabledModules) > 0
	// only allowed raw config adjustments here, otherwise use Adjust function
	cfg.Set(spNS("enabled"), c.Enabled, model.SourceAgentRuntime)

	return c, nil
}

// SetupOptionalDatadogConfigWithDir loads the datadog.yaml config file from a given config directory but will not fail on a missing file
func SetupOptionalDatadogConfigWithDir(configDir, configFile string) error {
	pkgconfigsetup.Datadog().AddConfigPath(configDir)
	if configFile != "" {
		pkgconfigsetup.Datadog().SetConfigFile(configFile)
	}
	// load the configuration
	_, err := pkgconfigsetup.LoadDatadogCustom(pkgconfigsetup.Datadog(), "datadog.yaml", optional.NewNoneOption[secrets.Component](), pkgconfigsetup.SystemProbe().GetEnvVars())
	// If `!failOnMissingFile`, do not issue an error if we cannot find the default config file.
	var e viper.ConfigFileNotFoundError
	if err != nil && !errors.As(err, &e) {
		// special-case permission-denied with a clearer error message
		if errors.Is(err, fs.ErrPermission) {
			if runtime.GOOS == "windows" {
				err = fmt.Errorf(`cannot access the Datadog config file (%w); try running the command in an Administrator shell"`, err)
			} else {
				err = fmt.Errorf("cannot access the Datadog config file (%w); try running the command under the same user as the Datadog Agent", err)
			}
		} else {
			err = fmt.Errorf("unable to load Datadog config file: %w", err)
		}
		return err
	}
	return nil
}
