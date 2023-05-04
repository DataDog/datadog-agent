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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

const (
	// Namespace is the top-level configuration key that all system-probe settings are nested underneath
	Namespace = "system_probe_config"
	spNS      = Namespace
	smNS      = "service_monitoring_config"
	dsmNS     = "data_streams_config"
	diNS      = "dynamic_instrumentation"

	defaultConnsMessageBatchSize = 600
	maxConnsMessageBatchSize     = 1000
)

// system-probe module names
const (
	NetworkTracerModule          ModuleName = "network_tracer"
	OOMKillProbeModule           ModuleName = "oom_kill_probe"
	TCPQueueLengthTracerModule   ModuleName = "tcp_queue_length_tracer"
	SecurityRuntimeModule        ModuleName = "security_runtime"
	ProcessModule                ModuleName = "process"
	EventMonitorModule           ModuleName = "event_monitor"
	DynamicInstrumentationModule ModuleName = "dynamic_instrumentation"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// Config represents the configuration options for the system-probe
type Config struct {
	Enabled             bool
	EnabledModules      map[ModuleName]struct{}
	ClosedSourceAllowed bool

	// When the system-probe is enabled in a separate container, we need a way to also disable the system-probe
	// packaged in the main agent container (without disabling network collection on the process-agent).
	ExternalSystemProbe bool

	SocketAddress      string
	MaxConnsPerMessage int

	LogFile          string
	LogLevel         string
	DebugPort        int
	TelemetryEnabled bool

	StatsdHost string
	StatsdPort int
}

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string) (*Config, error) {
	return newSysprobeConfig(configPath, true)
}

// NewCustom creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func NewCustom(configPath string, loadSecrets bool) (*Config, error) {
	return newSysprobeConfig(configPath, loadSecrets)
}

func newSysprobeConfig(configPath string, loadSecrets bool) (*Config, error) {
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
	_, err := aconfig.LoadCustom(aconfig.SystemProbe, "system-probe", loadSecrets, aconfig.Datadog.GetEnvVars())
	if err != nil {
		// System probe is not supported on darwin, so we should fail gracefully in this case.
		if runtime.GOOS != "darwin" {
			if errors.Is(err, os.ErrPermission) {
				log.Warnf("Error loading config: %v (check config file permissions for dd-agent user)", err)
			} else {
				log.Warnf("Error loading config: %v", err)
			}
		}

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

	c := &Config{
		Enabled:             cfg.GetBool(key(spNS, "enabled")),
		EnabledModules:      make(map[ModuleName]struct{}),
		ClosedSourceAllowed: isClosedSourceAllowed(),
		ExternalSystemProbe: cfg.GetBool(key(spNS, "external")),

		SocketAddress:      cfg.GetString(key(spNS, "sysprobe_socket")),
		MaxConnsPerMessage: cfg.GetInt(key(spNS, "max_conns_per_message")),

		LogFile:          cfg.GetString("log_file"),
		LogLevel:         cfg.GetString("log_level"),
		DebugPort:        cfg.GetInt(key(spNS, "debug_port")),
		TelemetryEnabled: cfg.GetBool(key(spNS, "telemetry_enabled")),

		StatsdHost: aconfig.GetBindHost(),
		StatsdPort: cfg.GetInt("dogstatsd_port"),
	}

	// backwards compatible log settings
	if !cfg.IsSet("log_level") && cfg.IsSet(key(spNS, "log_level")) {
		c.LogLevel = cfg.GetString(key(spNS, "log_level"))
		cfg.Set("log_level", c.LogLevel)
	}
	if !cfg.IsSet("log_file") && cfg.IsSet(key(spNS, "log_file")) {
		c.LogFile = cfg.GetString(key(spNS, "log_file"))
		cfg.Set("log_file", c.LogFile)
	}

	if c.MaxConnsPerMessage > maxConnsMessageBatchSize {
		log.Warn("Overriding the configured connections count per message limit because it exceeds maximum")
		c.MaxConnsPerMessage = defaultConnsMessageBatchSize
		cfg.Set(key(spNS, "max_conns_per_message"), c.MaxConnsPerMessage)
	}

	// this check must come first, so we can accurately tell if system_probe was explicitly enabled
	npmEnabled := cfg.GetBool("network_config.enabled")
	usmEnabled := cfg.GetBool(key(smNS, "enabled"))
	dsmEnabled := cfg.GetBool(key(dsmNS, "enabled"))

	if c.Enabled && !cfg.IsSet("network_config.enabled") && !usmEnabled && !dsmEnabled {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Info("`system_probe_config.enabled` is deprecated, enable NPM with `network_config.enabled` instead")
		// ensure others can key off of this single config value for NPM status
		cfg.Set("network_config.enabled", true)
		npmEnabled = true
	}

	if npmEnabled || usmEnabled || dsmEnabled {
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "enable_tcp_queue_length")) {
		c.EnabledModules[TCPQueueLengthTracerModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "enable_oom_kill")) {
		c.EnabledModules[OOMKillProbeModule] = struct{}{}
	}

	if cfg.GetBool("runtime_security_config.enabled") ||
		cfg.GetBool("runtime_security_config.fim_enabled") ||
		cfg.GetBool("event_monitoring_config.process.enabled") ||
		(c.ModuleIsEnabled(NetworkTracerModule) && cfg.GetBool("event_monitoring_config.network_process.enabled")) {
		c.EnabledModules[EventMonitorModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "process_config.enabled")) {
		c.EnabledModules[ProcessModule] = struct{}{}
	}

	if cfg.GetBool(key(diNS, "enabled")) {
		c.EnabledModules[DynamicInstrumentationModule] = struct{}{}
	}

	if len(c.EnabledModules) > 0 {
		c.Enabled = true
		if err := ValidateSocketAddress(c.SocketAddress); err != nil {
			log.Errorf("Could not parse %s.sysprobe_socket: %s", spNS, err)
			c.SocketAddress = defaultSystemProbeAddress
		}
	} else {
		c.Enabled = false
		c.SocketAddress = ""
	}

	cfg.Set(key(spNS, "sysprobe_socket"), c.SocketAddress)
	cfg.Set(key(spNS, "enabled"), c.Enabled)

	if cfg.GetBool(key(smNS, "process_service_inference", "enabled")) {
		if !usmEnabled && !dsmEnabled {
			log.Info("Both service monitoring and data streams monitoring are disabled, disabling process service inference")
			cfg.Set(key(smNS, "process_service_inference", "enabled"), false)
		} else {
			log.Info("process service inference is enabled")
		}
	}

	return c, nil
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}

// SetupOptionalDatadogConfig loads the datadog.yaml config file but will not fail on a missing file
func SetupOptionalDatadogConfig() error {
	return SetupOptionalDatadogConfigWithDir(defaultConfigDir, "")
}

// SetupOptionalDatadogConfig loads the datadog.yaml config file from a given config directory but will not fail on a missing file
func SetupOptionalDatadogConfigWithDir(configDir, configFile string) error {
	aconfig.Datadog.AddConfigPath(configDir)
	if configFile != "" {
		aconfig.Datadog.SetConfigFile(configFile)
	}
	// load the configuration
	_, err := aconfig.LoadDatadogCustomWithKnownEnvVars(aconfig.Datadog, "datadog.yaml", true, aconfig.SystemProbe.GetEnvVars())
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
