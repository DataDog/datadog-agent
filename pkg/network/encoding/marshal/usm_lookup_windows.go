// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// USMLookup determines the strategy for associating a given connection to USM
func USMLookup[K comparable, V any](c network.ConnectionStats, data map[types.ConnectionKey]*USMConnectionData[K, V]) *USMConnectionData[K, V] {
	for _, key := range network.ConnectionKeysFromConnectionStats(c) {
		if v, ok := data[key]; ok {
			return v
		}
	}

	return nil
}
