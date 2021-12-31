// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !android
// +build linux,!android

package netlink

import "github.com/DataDog/datadog-agent/pkg/network"

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
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
