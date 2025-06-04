// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && !linux_bpf

package module

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"

type nopNetworkCollector struct{}

func newNetworkCollector(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
	return &nopNetworkCollector{}, nil
}

func (c *nopNetworkCollector) Close() {
}

func (c *nopNetworkCollector) GetStats(_ core.PidSet) (map[uint32]core.NetworkStats, error) {
	return nil, nil
}
