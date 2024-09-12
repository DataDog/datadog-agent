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
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
)

const discoveryNS = "discovery"

type discoveryConfig struct {
	cpuUsageUpdateDelay time.Duration
}

func newConfig() *discoveryConfig {
	cfg := ddconfig.SystemProbe()
	sysconfig.Adjust(cfg)

	return &discoveryConfig{
		cpuUsageUpdateDelay: cfg.GetDuration(join(discoveryNS, "cpu_usage_update_delay")),
	}
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}
