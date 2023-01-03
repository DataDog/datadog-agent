// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"os"
	"strings"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AgentConfig is the global config for the process-agent. This information
// is sourced from config files and the environment variables.
// AgentConfig is shared across process-agent checks and should only contain shared objects and
// settings that cannot be read directly from the global Config object.
// For any other setting, use `pkg/config`.
type AgentConfig struct {
	MaxConnsPerMessage int

	// System probe collection configuration
	SystemProbeAddress string
}

// NewDefaultAgentConfig returns an AgentConfig with defaults initialized
func NewDefaultAgentConfig() *AgentConfig {
	ac := &AgentConfig{
		MaxConnsPerMessage: 600,
		// System probe collection configuration
		SystemProbeAddress: defaultSystemProbeAddress,
	}

	// Set default values for proc/sys paths if unset.
	// Don't set this is /host is not mounted to use context within container.
	// Generally only applicable for container-only cases like Fargate.
	if config.IsContainerized() && util.PathExists("/host") {
		if v := os.Getenv("HOST_PROC"); v == "" {
			os.Setenv("HOST_PROC", "/host/proc")
		}
		if v := os.Getenv("HOST_SYS"); v == "" {
			os.Setenv("HOST_SYS", "/host/sys")
		}
	}

	return ac
}

// LoadConfigIfExists takes a path to either a directory containing datadog.yaml or a direct path to a datadog.yaml file
// and loads it into ddconfig.Datadog. It does this silently, and does not produce any logs.
func LoadConfigIfExists(path string) error {
	if path != "" {
		if util.PathExists(path) {
			config.Datadog.AddConfigPath(path)
			if strings.HasSuffix(path, ".yaml") { // If they set a config file directly, let's try to honor that
				config.Datadog.SetConfigFile(path)
			}

			if _, err := config.LoadWithoutSecret(); err != nil {
				return err
			}
		} else {
			log.Infof("no config exists at %s, ignoring...", path)
		}
	}
	return nil
}

// NewAgentConfig returns an AgentConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewAgentConfig(loggerName config.LoggerName, yamlPath string, syscfg *sysconfig.Config) (*AgentConfig, error) {
	cfg := NewDefaultAgentConfig()
	if err := cfg.LoadAgentConfig(yamlPath); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration
	logFile := config.Datadog.GetString("process_config.log_file")
	if err := setupLogger(loggerName, logFile); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	if syscfg.Enabled {
		cfg.MaxConnsPerMessage = syscfg.MaxConnsPerMessage
		cfg.SystemProbeAddress = syscfg.SocketAddress
	}

	return cfg, nil
}

// InitRuntimeSettings registers settings to be added to the runtime config.
func InitRuntimeSettings() {
	// NOTE: Any settings you want to register should simply be added here
	processRuntimeSettings := []settings.RuntimeSetting{
		settings.LogLevelRuntimeSetting{},
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		err := settings.RegisterRuntimeSetting(setting)
		if err != nil {
			_ = log.Warnf("cannot initialize the runtime setting %s: %v", setting.Name(), err)
		}
	}
}

// loadEnvVariables reads env variables specific to process-agent and overrides the corresponding settings
// in the global Config object.
// This function is used to handle historic process-agent env vars. New settings should be
// handled directly in the /pkg/config/process.go file
func loadEnvVariables() {
	// The following environment variables will be loaded in the order listed, meaning variables
	// further down the list may override prior variables.
	for _, variable := range []struct{ env, cfg string }{
		{"DD_ORCHESTRATOR_URL", "orchestrator_explorer.orchestrator_dd_url"},
		{"HTTPS_PROXY", "proxy.https"},
	} {
		if v, ok := os.LookupEnv(variable.env); ok {
			config.Datadog.Set(variable.cfg, v)
		}
	}

	if v := os.Getenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"); v != "" {
		endpoints := make(map[string][]string)
		if err := json.Unmarshal([]byte(v), &endpoints); err != nil {
			log.Errorf(`Could not parse DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS: %v. It must be of the form '{"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'.`, err)
		} else {
			config.Datadog.Set("orchestrator_explorer.orchestrator_additional_endpoints", endpoints)
		}
	}
}

func setupLogger(loggerName config.LoggerName, logFile string) error {
	if config.Datadog.GetBool("disable_file_logging") {
		logFile = ""
	}

	return config.SetupLogger(
		loggerName,
		config.Datadog.GetString("log_level"),
		logFile,
		config.GetSyslogURI(),
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
}
