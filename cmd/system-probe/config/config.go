package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/viper"
)

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

const (
	// Namespace is the top-level configuration key that all system-probe settings are nested underneath
	Namespace             = "system_probe_config"
	spNS                  = Namespace
	defaultConfigFileName = "system-probe.yaml"

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

	ProfilingEnabled        bool
	ProfilingSite           string
	ProfilingURL            string
	ProfilingEnvironment    string
	ProfilingPeriod         time.Duration
	ProfilingCPUDuration    time.Duration
	ProfilingMutexFraction  int
	ProfilingBlockRate      int
	ProfilingWithGoroutines bool
}

// New creates a config object for system-probe. It assumes no configuration has been loaded as this point.
func New(configPath string) (*Config, error) {
	aconfig.InitSystemProbeConfig(aconfig.Datadog)
	aconfig.Datadog.SetConfigName("system-probe")
	// set the paths where a config file is expected
	if len(configPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		aconfig.Datadog.AddConfigPath(configPath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(configPath, ".yaml") {
			aconfig.Datadog.SetConfigFile(configPath)
		}
	}
	aconfig.Datadog.AddConfigPath(defaultConfigDir)

	_, err := aconfig.LoadWithoutSecret()
	var e viper.ConfigFileNotFoundError
	if err != nil {
		if errors.As(err, &e) || errors.Is(err, os.ErrNotExist) {
			log.Infof("no config exists at %s, ignoring...", configPath)
		} else {
			return nil, err
		}
	}
	return load(configPath)
}

// Merge will merge the system-probe configuration into the existing datadog configuration
func Merge(configPath string) (*Config, error) {
	aconfig.InitSystemProbeConfig(aconfig.Datadog)
	if configPath != "" {
		if !strings.HasSuffix(configPath, ".yaml") {
			configPath = path.Join(configPath, defaultConfigFileName)
		}
	} else {
		configPath = path.Join(defaultConfigDir, defaultConfigFileName)
	}

	if f, err := os.Open(configPath); err == nil {
		err = aconfig.Datadog.MergeConfig(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("error merging system-probe config file: %s", err)
		}
	} else {
		log.Infof("no config exists at %s, ignoring...", configPath)
	}

	return load(configPath)
}

func load(configPath string) (*Config, error) {
	cfg := aconfig.Datadog

	if err := aconfig.ResolveSecrets(cfg, filepath.Base(configPath)); err != nil {
		return nil, err
	}

	c := &Config{
		Enabled:             cfg.GetBool(key(spNS, "enabled")),
		EnabledModules:      make(map[ModuleName]struct{}),
		ExternalSystemProbe: cfg.GetBool(key(spNS, "external")),

		SocketAddress:      cfg.GetString(key(spNS, "sysprobe_socket")),
		MaxConnsPerMessage: cfg.GetInt(key(spNS, "max_conns_per_message")),

		LogFile:   cfg.GetString(key(spNS, "log_file")),
		LogLevel:  cfg.GetString(key(spNS, "log_level")),
		DebugPort: cfg.GetInt(key(spNS, "debug_port")),

		StatsdHost: aconfig.GetBindHost(),
		StatsdPort: cfg.GetInt("dogstatsd_port"),

		ProfilingEnabled:        cfg.GetBool(key(spNS, "internal_profiling.enabled")),
		ProfilingSite:           cfg.GetString(key(spNS, "internal_profiling.site")),
		ProfilingURL:            cfg.GetString(key(spNS, "internal_profiling.profile_dd_url")),
		ProfilingEnvironment:    cfg.GetString(key(spNS, "internal_profiling.env")),
		ProfilingPeriod:         cfg.GetDuration(key(spNS, "internal_profiling.period")),
		ProfilingCPUDuration:    cfg.GetDuration(key(spNS, "internal_profiling.cpu_duration")),
		ProfilingMutexFraction:  cfg.GetInt(key(spNS, "internal_profiling.mutex_profile_fraction")),
		ProfilingBlockRate:      cfg.GetInt(key(spNS, "internal_profiling.block_profile_rate")),
		ProfilingWithGoroutines: cfg.GetBool(key(spNS, "internal_profiling.enable_goroutine_stacktraces")),
	}

	if err := ValidateSocketAddress(c.SocketAddress); err != nil {
		log.Errorf("Could not parse %s.sysprobe_socket: %s", spNS, err)
		c.SocketAddress = defaultSystemProbeAddress
		cfg.Set(key(spNS, "sysprobe_socket"), c.SocketAddress)
	}

	if c.MaxConnsPerMessage > maxConnsMessageBatchSize {
		log.Warn("Overriding the configured connections count per message limit because it exceeds maximum")
		c.MaxConnsPerMessage = defaultConnsMessageBatchSize
		cfg.Set(key(spNS, "max_conns_per_message"), c.MaxConnsPerMessage)
	}

	// this check must come first so we can accurately tell if system_probe was explicitly enabled
	if cfg.GetBool("network_config.enabled") {
		log.Info("network_config.enabled detected: enabling system-probe with network module running.")
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	} else if c.Enabled && !cfg.IsSet("network_config.enabled") {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Info("network_config not found, but system-probe was enabled, enabling network module by default")
		c.EnabledModules[NetworkTracerModule] = struct{}{}
	}

	if cfg.GetBool(key(spNS, "enable_tcp_queue_length")) {
		log.Info("system_probe_config.enable_tcp_queue_length detected, will enable system-probe with TCP queue length check")
		c.EnabledModules[TCPQueueLengthTracerModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "enable_oom_kill")) {
		log.Info("system_probe_config.enable_oom_kill detected, will enable system-probe with OOM Kill check")
		c.EnabledModules[OOMKillProbeModule] = struct{}{}
	}
	if cfg.GetBool("runtime_security_config.enabled") || cfg.GetBool("runtime_security_config.fim_enabled") {
		log.Info("runtime_security_config.enabled or runtime_security_config.fim_enabled detected, enabling system-probe")
		c.EnabledModules[SecurityRuntimeModule] = struct{}{}
	}
	if cfg.GetBool(key(spNS, "process_config.enabled")) {
		log.Info("process_config.enabled detected, enabling system-probe")
		c.EnabledModules[ProcessModule] = struct{}{}
	}

	if len(c.EnabledModules) > 0 {
		c.Enabled = true
	}

	return c, nil
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}
