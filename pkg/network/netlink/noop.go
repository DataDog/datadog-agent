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

//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	return nil
}

//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) DeleteTranslation(c network.ConnectionStats) {

}

//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) IsSampling() bool {
	return false
}

//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) Close() {}

//nolint:revive // TODO(NET) Fix revive linter
func (c *noOpConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]DebugConntrackEntry, error) {
	return nil, nil
}

// Describe returns all descriptions of the collector
//
//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) Describe(ch chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
//
//nolint:revive // TODO(NET) Fix revive linter
func (*noOpConntracker) Collect(ch chan<- prometheus.Metric) {}
