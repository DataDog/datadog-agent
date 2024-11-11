// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
}

// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
func (*noOpConntracker) GetType() string { return "" }

func (*noOpConntracker) GetTranslationForConn(_c *network.ConnectionTuple) *network.IPTranslation {
	return nil
}

func (*noOpConntracker) DeleteTranslation(_c *network.ConnectionTuple) {

}

func (*noOpConntracker) IsSampling() bool {
	return false
}

func (*noOpConntracker) Close() {}

func (*noOpConntracker) DumpCachedTable(_ctx context.Context) (map[uint32][]DebugConntrackEntry, error) {
	return nil, nil
}

// Describe returns all descriptions of the collector
func (*noOpConntracker) Describe(_ch chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (*noOpConntracker) Collect(_ch chan<- prometheus.Metric) {}
