// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"os"
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// Config holds the configuration for the runtime security agent
type Config struct {
	// RuntimeEnabled defines if the runtime security module should be enabled
	RuntimeEnabled bool
	// PoliciesDir defines the folder in which the policy files are located
	PoliciesDir string
	// WatchPoliciesDir activate policy dir inotify
	WatchPoliciesDir bool
	// PolicyMonitorEnabled enable policy monitoring
	PolicyMonitorEnabled bool
	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int
	// EventServerRate defines the grpc server rate at which events can be sent
	EventServerRate int
	// EventServerRetention defines an event retention period so that some fields can be resolved
	EventServerRetention int
	// FIMEnabled determines whether fim rules will be loaded
	FIMEnabled bool
	// SelfTestEnabled defines if the self tests should be executed at startup or not
	SelfTestEnabled bool
	// SelfTestSendReport defines if a self test event will be emitted
	SelfTestSendReport bool
	// RemoteConfigurationEnabled defines whether to use remote monitoring
	RemoteConfigurationEnabled bool
	// LogPatterns pattern to be used by the logger for trace level
	LogPatterns []string
	// LogTags tags to be used by the logger for trace level
	LogTags []string
	// HostServiceName string
	HostServiceName string
}

func setEnv() {
	if coreconfig.IsContainerized() && util.PathExists("/host") {
		if v := os.Getenv("HOST_PROC"); v == "" {
			os.Setenv("HOST_PROC", "/host/proc")
		}
		if v := os.Getenv("HOST_SYS"); v == "" {
			os.Setenv("HOST_SYS", "/host/sys")
		}
	}
}

// NewConfig returns a new Config object
func NewConfig() *Config {
	c := &Config{
		RuntimeEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.enabled"),
		FIMEnabled:     coreconfig.SystemProbe.GetBool("runtime_security_config.fim_enabled"),

		SelfTestEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.enabled"),
		SelfTestSendReport:         coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.send_report"),
		RemoteConfigurationEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.remote_configuration.enabled"),

		EventServerBurst:     coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:      coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention: coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.retention"),

		// policy & ruleset
		PoliciesDir:          coreconfig.SystemProbe.GetString("runtime_security_config.policies.dir"),
		WatchPoliciesDir:     coreconfig.SystemProbe.GetBool("runtime_security_config.policies.watch_dir"),
		PolicyMonitorEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.policies.monitor.enabled"),

		LogPatterns: coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_patterns"),
		LogTags:     coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_tags"),
	}

	c.sanitize()

	setEnv()
	return c
}

// IsRuntimeEnabled returns true if any feature is enabled
func (c *Config) IsRuntimeEnabled() bool {
	return c.RuntimeEnabled || c.FIMEnabled
}

// sanitize ensures that the configuration is properly setup
func (c *Config) sanitize() {
	// if runtime is enabled then we force fim
	if c.RuntimeEnabled {
		c.FIMEnabled = true
	}

	serviceName := utils.GetTagValue("service", coreconfig.GetGlobalConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}
}
