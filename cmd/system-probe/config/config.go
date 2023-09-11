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
	"runtime"
	"strings"

	"github.com/DataDog/viper"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

const (
	// Namespace is the top-level configuration key that all system-probe settings are nested underneath
	Namespace = "system_probe_config"
)

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

	StatsdHost string
	StatsdPort int

	GRPCServerEnabled bool
}

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string) (*Config, error) {
	return newSysprobeConfig(configPath)
}

func newSysprobeConfig(configPath string) (*Config, error) {
	// System probe is not supported on darwin, so we should fail gracefully in this case.
	if runtime.GOOS == "darwin" {
		return &Config{}, nil
	}

	aconfig.SystemProbe.SetConfigName("system-probe")
	// set the paths where a config file is expected
	if len(configPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first, so it's first in line
		aconfig.SystemProbe.AddConfigPath(configPath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(configPath, ".yaml") {
			aconfig.SystemProbe.SetConfigFile(configPath)
		}
	} else {
		// only add default if a custom configPath was not supplied
		aconfig.SystemProbe.AddConfigPath(defaultConfigDir)
	}
	// load the configuration
	_, err := aconfig.LoadCustom(aconfig.SystemProbe, "system-probe", false, aconfig.Datadog.GetEnvVars())
	if err != nil {
		var e viper.ConfigFileNotFoundError
		if errors.As(err, &e) || errors.Is(err, os.ErrNotExist) {
			// do nothing, we can ignore a missing system-probe.yaml config file
		} else if errors.Is(err, fs.ErrPermission) {
			// special-case permission-denied with a clearer error message
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf(`cannot access the system-probe config file (%w); try running the command in an Administrator shell"`, err)
			} else {
				return nil, fmt.Errorf("cannot access the system-probe config file (%w); try running the command under the same user as the Datadog Agent", err)
			}
		} else {
			return nil, fmt.Errorf("unable to load system-probe config file: %w", err)
		}
	}
	return load()
}

func load() (*Config, error) {
	cfg := aconfig.SystemProbe
	Adjust(cfg)

	c := &Config{
		Enabled:             cfg.GetBool(spNS("enabled")),
		EnabledModules:      make(map[ModuleName]struct{}),
		ExternalSystemProbe: cfg.GetBool(spNS("external")),

		SocketAddress:      cfg.GetString(spNS("sysprobe_socket")),
		GRPCServerEnabled:  cfg.GetBool(spNS("grpc_enabled")),
		MaxConnsPerMessage: cfg.GetInt(spNS("max_conns_per_message")),

		LogFile:          cfg.GetString("log_file"),
		LogLevel:         cfg.GetString("log_level"),
		DebugPort:        cfg.GetInt(spNS("debug_port")),
		HealthPort:       cfg.GetInt(spNS("health_port")),
		TelemetryEnabled: cfg.GetBool(spNS("telemetry_enabled")),

		StatsdHost: aconfig.GetBindHost(),
		StatsdPort: cfg.GetInt("dogstatsd_port"),
	}

	npmEnabled := cfg.GetBool(netNS("enabled"))
	usmEnabled := cfg.GetBool(smNS("enabled"))
	dsmEnabled := cfg.GetBool(dsmNS("enabled"))

	if npmEnabled || usmEnabled || dsmEnabled {
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
	if cfg.GetBool(spNS("process_config.enabled")) {
		c.EnabledModules[ProcessModule] = struct{}{}
	}
	if cfg.GetBool(diNS("enabled")) {
		c.EnabledModules[DynamicInstrumentationModule] = struct{}{}
	}
	if cfg.GetBool(nskey("ebpf_check", "enabled")) {
		c.EnabledModules[EBPFModule] = struct{}{}
	}
	if cfg.GetBool("system_probe_config.language_detection.enabled") {
		c.EnabledModules[LanguageDetectionModule] = struct{}{}
	}

	if cfg.GetBool(wcdNS("enabled")) {
		c.EnabledModules[WindowsCrashDetectModule] = struct{}{}
	}
	if runtime.GOOS == "windows" {
		if c.ModuleIsEnabled(NetworkTracerModule) {
			// enable the windows crash detection module if the network tracer
			// module is enabled, to allow the core agent to detect our own crash
			c.EnabledModules[WindowsCrashDetectModule] = struct{}{}
		}
	}

	c.Enabled = len(c.EnabledModules) > 0
	// only allowed raw config adjustments here, otherwise use Adjust function
	cfg.Set(spNS("enabled"), c.Enabled)

	return c, nil
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}

// SetupOptionalDatadogConfigWithDir loads the datadog.yaml config file from a given config directory but will not fail on a missing file
func SetupOptionalDatadogConfigWithDir(configDir, configFile string) error {
	aconfig.Datadog.AddConfigPath(configDir)
	if configFile != "" {
		aconfig.Datadog.SetConfigFile(configFile)
	}
	// load the configuration
	_, err := aconfig.LoadDatadogCustom(aconfig.Datadog, "datadog.yaml", false, aconfig.SystemProbe.GetEnvVars())
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
