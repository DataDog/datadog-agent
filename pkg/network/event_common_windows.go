// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// ConnectionKeysFromConnectionStats constructs connection key using the underlying raw connection stats object, which is produced by the tracer.
func ConnectionKeysFromConnectionStats(connectionStats ConnectionStats) []types.ConnectionKey {

	// USM data is always indexed as (client, server), but we don't know which is the remote
	// and which is the local address. To account for this, we'll construct 2 possible
	// connection keys and check for both of them in the aggregations map.
	connectionKeys := []types.ConnectionKey{
		types.NewConnectionKey(connectionStats.Source, connectionStats.Dest, connectionStats.SPort, connectionStats.DPort),
	}

	return connectionKeys
}
