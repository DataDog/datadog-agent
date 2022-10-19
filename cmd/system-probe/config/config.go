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
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

const (
	// Namespace is the top-level configuration key that all system-probe settings are nested underneath
	Namespace = "system_probe_config"
	spNS      = Namespace

	defaultConnsMessageBatchSize = 600
	maxConnsMessageBatchSize     = 1000
)

// system-probe module names
const (
	NetworkTracerModule        ModuleName = "network_tracer"
	OOMKillProbeModule         ModuleName = "oom_kill_probe"
	TCPQueueLengthTracerModule ModuleName = "tcp_queue_length_tracer"
	SecurityRuntimeModule      ModuleName = "security_runtime"
	ProcessModule              ModuleName = "process"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// Config represents the configuration options for the system-probe
type Config struct {
	Enabled        bool
	EnabledModules map[ModuleName]struct{}

	// When the system-probe is enabled in a separate container, we need a way to also disable the system-probe
	// packaged in the main agent container (without disabling network collection on the process-agent).
	ExternalSystemProbe bool

	SocketAddress      string
	MaxConnsPerMessage int

	LogFile   string
	LogLevel  string
	DebugPort int

	StatsdHost string
	StatsdPort int

	// Settings for profiling, or nil if not enabled
	ProfilingSettings *profiling.Settings
}

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string) (*Config, error) {
	aconfig.SystemProbe.SetConfigName("system-probe")
	// set the paths where a config file is expected
	if len(configPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		aconfig.SystemProbe.AddConfigPath(configPath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(configPath, ".yaml") {
			aconfig.SystemProbe.SetConfigFile(configPath)
		}
	}
	aconfig.SystemProbe.AddConfigPath(defaultConfigDir)
	// load the configuration
	_, err := aconfig.LoadCustom(aconfig.SystemProbe, "system-probe", true)
	// If `!failOnMissingFile`, do not issue an error if we cannot find the default config file.
	var e viper.ConfigFileNotFoundError
	if err != nil && (!errors.As(err, &e) || configPath != "") {
		// special-case permission-denied with a clearer error message
		if errors.Is(err, fs.ErrPermission) {
			if runtime.GOOS == "windows" {
				err = fmt.Errorf(`cannot access the system-probe config file (%w); try running the command in an Administrator shell"`, err)
			} else {
				err = fmt.Errorf("cannot access the system-probe config file (%w); try running the command under the same user as the Datadog Agent", err)
			}
		} else {
			err = fmt.Errorf("unable to load system-probe config file: %w", err)
		}
		return nil, err
	}
	return load()
}

func load() (*Config, error) {
	cfg := aconfig.SystemProbe

	var profSettings *profiling.Settings
	if cfg.GetBool(key(spNS, "internal_profiling.enabled")) {
		v, _ := version.Agent()

		var site string
		cfgSite := cfg.GetString(key(spNS, "internal_profiling.site"))
		cfgURL := cfg.GetString(key(spNS, "internal_profiling.profile_dd_url"))
		// check if TRACE_AGENT_URL is set, in which case, forward the profiles to the trace agent
		if traceAgentURL := os.Getenv("TRACE_AGENT_URL"); len(traceAgentURL) > 0 {
			site = fmt.Sprintf(profiling.ProfilingLocalURLTemplate, traceAgentURL)
		} else {
			site = fmt.Sprintf(profiling.ProfilingURLTemplate, cfgSite)
			if cfgURL != "" {
				site = cfgURL
			}
		}

		profSettings = &profiling.Settings{
			ProfilingURL:         site,
			Env:                  cfg.GetString(key(spNS, "internal_profiling.env")),
			Service:              "system-probe",
			Period:               cfg.GetDuration(key(spNS, "internal_profiling.period")),
			CPUDuration:          cfg.GetDuration(key(spNS, "internal_profiling.cpu_duration")),
			MutexProfileFraction: cfg.GetInt(key(spNS, "internal_profiling.mutex_profile_fraction")),
			BlockProfileRate:     cfg.GetInt(key(spNS, "internal_profiling.block_profile_rate")),
			WithGoroutineProfile: cfg.GetBool(key(spNS, "internal_profiling.enable_goroutine_stacktraces")),
			Tags:                 []string{fmt.Sprintf("version:%v", v)},
		}
	}
	c := &Config{
		Enabled:             cfg.GetBool(key(spNS, "enabled")),
		EnabledModules:      make(map[ModuleName]struct{}),
		ExternalSystemProbe: cfg.GetBool(key(spNS, "external")),

		SocketAddress:      cfg.GetString(key(spNS, "sysprobe_socket")),
		MaxConnsPerMessage: cfg.GetInt(key(spNS, "max_conns_per_message")),

		LogFile:   cfg.GetString("log_file"),
		LogLevel:  cfg.GetString("log_level"),
		DebugPort: cfg.GetInt(key(spNS, "debug_port")),

		StatsdHost: aconfig.GetBindHost(),
		StatsdPort: cfg.GetInt("dogstatsd_port"),

		ProfilingSettings: profSettings,
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
	usmEnabled := cfg.GetBool("service_monitoring_config.enabled")

	if c.Enabled && !cfg.IsSet("network_config.enabled") && !usmEnabled {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Info("`system_probe_config.enabled` is deprecated, enable NPM with `network_config.enabled` instead")
		// ensure others can key off of this single config value for NPM status
		cfg.Set("network_config.enabled", true)
		npmEnabled = true
	}

	if npmEnabled || usmEnabled {
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "enable_tcp_queue_length")) {
		c.EnabledModules[TCPQueueLengthTracerModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "enable_oom_kill")) {
		c.EnabledModules[OOMKillProbeModule] = struct{}{}
	}
	if cfg.GetBool("runtime_security_config.enabled") || cfg.GetBool("runtime_security_config.fim_enabled") || cfg.GetBool("runtime_security_config.event_monitoring.enabled") {
		c.EnabledModules[SecurityRuntimeModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "process_config.enabled")) {
		c.EnabledModules[ProcessModule] = struct{}{}
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

	return c, nil
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}
