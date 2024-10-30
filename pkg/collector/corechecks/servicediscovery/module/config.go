// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"strings"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const discoveryNS = "discovery"

type discoveryConfig struct {
	cpuUsageUpdateDelay time.Duration
	ignoreComms         map[string]struct{}
}

func newConfig() *discoveryConfig {
	cfg := ddconfig.SystemProbe()
	sysconfig.Adjust(cfg)

	conf := &discoveryConfig{
		cpuUsageUpdateDelay: cfg.GetDuration(join(discoveryNS, "cpu_usage_update_delay")),
	}

	conf.loadIgnoredComms(cfg.GetStringSlice(join(discoveryNS, "ignored_command_names")))

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

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}
