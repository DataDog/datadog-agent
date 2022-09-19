// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package netlink

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
}

func (*noOpConntracker) Start() error {
	return nil
}

func (*noOpConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	return nil
}

func (*noOpConntracker) DeleteTranslation(c network.ConnectionStats) {

}

func (*noOpConntracker) IsSampling() bool {
	return false
}

func (*noOpConntracker) Close() {}

func (*noOpConntracker) GetStats() map[string]int64 {
	return map[string]int64{
		"noop_conntracker": 0,
	}
}

func (c *noOpConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]DebugConntrackEntry, error) {
	return nil, nil
}
