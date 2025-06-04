// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"strings"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const discoveryNS = "discovery"

type discoveryConfig struct {
	ebpf.Config
	cpuUsageUpdateDelay time.Duration
	networkStatsEnabled bool
	networkStatsPeriod  time.Duration
	ignoreComms         map[string]struct{}
	ignoreServices      map[string]struct{}
}

func newConfig() *discoveryConfig {
	cfg := ddconfig.SystemProbe()
	sysconfig.Adjust(cfg)

	conf := &discoveryConfig{
		Config:              *ebpf.NewConfig(),
		cpuUsageUpdateDelay: cfg.GetDuration(join(discoveryNS, "cpu_usage_update_delay")),
		networkStatsEnabled: cfg.GetBool(join(discoveryNS, "network_stats.enabled")),
		networkStatsPeriod:  cfg.GetDuration(join(discoveryNS, "network_stats.period")),
	}

	conf.loadIgnoredComms(cfg.GetStringSlice(join(discoveryNS, "ignored_command_names")))
	conf.loadIgnoredServices(cfg.GetStringSlice(join(discoveryNS, "ignored_services")))

	return conf
}

// loadIgnoredComms read process names that should not be reported as a service from input string
func (config *discoveryConfig) loadIgnoredComms(comms []string) {
	if len(comms) == 0 {
		log.Warn("loading ignored commands found empty commands list")
		return
	}
	config.ignoreComms = make(map[string]struct{}, len(comms))

	for _, comm := range comms {
		if len(comm) > maxCommLen {
			config.ignoreComms[comm[:maxCommLen]] = struct{}{}
			log.Warnf("truncating command name %q has %d bytes to %d", comm, len(comm), maxCommLen)
		} else if len(comm) > 0 {
			config.ignoreComms[comm] = struct{}{}
		}
	}
}

// loadIgnoredServices saves names that should not be reported as a service
func (config *discoveryConfig) loadIgnoredServices(services []string) {
	if len(services) == 0 {
		log.Debug("loading ignored services found empty services list")
		return
	}
	config.ignoreServices = make(map[string]struct{}, len(services))

	for _, service := range services {
		config.ignoreServices[service] = struct{}{}
	}
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}
