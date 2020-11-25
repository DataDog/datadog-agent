// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"fmt"
	"time"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// Config holds the configuration for the runtime security agent
type Config struct {
	ebpf.Config

	// Enabled defines if the runtime security module should be enabled
	Enabled bool
	// PoliciesDir defines the folder in which the policy files are located
	PoliciesDir string
	// EnableKernelFilters defines if in-kernel filtering should be activated or not
	EnableKernelFilters bool
	// EnableApprovers defines if in-kernel approvers should be activated or not
	EnableApprovers bool
	// EnableDiscarders defines if in-kernel discarders should be activated or not
	EnableDiscarders bool
	// FlushDiscarderWindow defines the maximum time window for discarders removal.
	// This is used during reload to avoid removing all the discarders at the same time.
	FlushDiscarderWindow int
	// SocketPath is the path to the socket that is used to communicate with the security agent
	SocketPath string
	// SyscallMonitor defines if the syscall monitor should be activated or not
	SyscallMonitor bool
	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int
	// EventServerRate defines the grpc server rate at which events can be sent
	EventServerRate int
	// PIDCacheSize is the size of the user space PID caches
	PIDCacheSize int
	// LoadControllerEventsCountThreshold defines the amount of events past which we will trigger the in-kernel circuit breaker
	LoadControllerEventsCountThreshold int64
	// LoadControllerDiscarderTimeout defines the amount of time discarders set by the load controller should last
	LoadControllerDiscarderTimeout time.Duration
	// LoadControllerControlPeriod defines the period at which the load controller will empty the user space counter used
	// to evaluate the amount of events brought back to user space
	LoadControllerControlPeriod time.Duration
	// StatsAddr defines the statsd address
	StatsdAddr string
}

// NewConfig returns a new Config object
func NewConfig(cfg *config.AgentConfig) (*Config, error) {
	c := &Config{
		Config:                             *ebpf.SysProbeConfigFromConfig(cfg),
		Enabled:                            aconfig.Datadog.GetBool("runtime_security_config.enabled"),
		EnableKernelFilters:                aconfig.Datadog.GetBool("runtime_security_config.enable_kernel_filters"),
		EnableApprovers:                    aconfig.Datadog.GetBool("runtime_security_config.enable_approvers"),
		EnableDiscarders:                   aconfig.Datadog.GetBool("runtime_security_config.enable_discarders"),
		FlushDiscarderWindow:               aconfig.Datadog.GetInt("runtime_security_config.flush_discarder_window"),
		SocketPath:                         aconfig.Datadog.GetString("runtime_security_config.socket"),
		SyscallMonitor:                     aconfig.Datadog.GetBool("runtime_security_config.syscall_monitor.enabled"),
		PoliciesDir:                        aconfig.Datadog.GetString("runtime_security_config.policies.dir"),
		EventServerBurst:                   aconfig.Datadog.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:                    aconfig.Datadog.GetInt("runtime_security_config.event_server.rate"),
		PIDCacheSize:                       aconfig.Datadog.GetInt("runtime_security_config.pid_cache_size"),
		LoadControllerEventsCountThreshold: int64(aconfig.Datadog.GetInt("runtime_security_config.load_controller.events_count_threshold")),
		LoadControllerDiscarderTimeout:     time.Duration(aconfig.Datadog.GetInt("runtime_security_config.load_controller.discarder_timeout")) * time.Second,
		LoadControllerControlPeriod:        time.Duration(aconfig.Datadog.GetInt("runtime_security_config.load_controller.control_period")) * time.Second,
		StatsdAddr:                         fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort),
	}

	if !c.Enabled {
		return c, nil
	}

	if !aconfig.Datadog.IsSet("runtime_security_config.enable_approvers") && c.EnableKernelFilters {
		c.EnableApprovers = true
	}

	if !aconfig.Datadog.IsSet("runtime_security_config.enable_discarders") && c.EnableKernelFilters {
		c.EnableDiscarders = true
	}

	if !c.EnableApprovers && !c.EnableDiscarders {
		c.EnableKernelFilters = false
	}

	return c, nil
}
