// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// USMLookup determines the strategy for associating a given connection to USM
// In the context of Linux we may perform up to 4 lookups as described below
func USMLookup[K comparable, V any](c network.ConnectionStats, data map[types.ConnectionKey]*USMConnectionData[K, V]) *USMConnectionData[K, V] {
	var connectionData *USMConnectionData[K, V]

	// WithKey will attempt 4 lookups in total
	// 1) (A, B)
	// 2) (B, A)
	// 3) (translated(A), translated(B))
	// 3) (translated(B), translated(A))
	// The callback API is used to avoid allocating a slice of all pre-computed keys
	network.WithKey(c, func(key types.ConnectionKey) (stopIteration bool) {
		val, ok := data[key]
		if !ok {
			return false
		}

		connectionData = val
		return true
	})

	return connectionData
}
