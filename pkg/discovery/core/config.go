// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package core provides the core functionality for service discovery.
package core

import (
	"strings"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	discoveryNS = "discovery"
	// MaxCommLen is maximum command name length to process when checking for non-reportable commands,
	// is one byte less (excludes end of line) than the maximum of /proc/<pid>/comm
	// defined in https://man7.org/linux/man-pages/man5/proc.5.html.
	MaxCommLen = 15
)

// DiscoveryConfig holds the configuration for service discovery.
type DiscoveryConfig struct {
	IgnoreComms map[string]struct{}
}

// NewConfig creates a new DiscoveryConfig with default values.
func NewConfig() *DiscoveryConfig {
	cfg := ddconfig.SystemProbe()
	sysconfig.Adjust(cfg)

	conf := &DiscoveryConfig{}

	conf.loadIgnoredComms(cfg.GetStringSlice(join(discoveryNS, "ignored_command_names")))

	return conf
}

// loadIgnoredComms read process names that should not be reported as a service from input string
func (config *DiscoveryConfig) loadIgnoredComms(comms []string) {
	if len(comms) == 0 {
		log.Warn("loading ignored commands found empty commands list")
		return
	}
	config.IgnoreComms = make(map[string]struct{}, len(comms))

	for _, comm := range comms {
		if len(comm) > MaxCommLen {
			config.IgnoreComms[comm[:MaxCommLen]] = struct{}{}
			log.Warnf("truncating command name %q has %d bytes to %d", comm, len(comm), MaxCommLen)
		} else if len(comm) > 0 {
			config.IgnoreComms[comm] = struct{}{}
		}
	}
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}
