// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package debugging provides debug-friendly representation of internal data structures
package debugging

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// address represents represents a IP:Port
type address struct {
	IP   string
	Port uint16
}

// key represents a (client, server, table name) tuple.
type key struct {
	Client address
	Server address
}

// RequestSummary represents a (debug-friendly) aggregated view of requests
type RequestSummary struct {
	key
}

// Redis returns a debug-friendly representation of map[postgres.Key]postgres.RequestStats
func Redis(stats map[redis.Key]*redis.RequestStat) []RequestSummary {
	all := make([]RequestSummary, 0)
	for k := range stats {
		clientAddr := formatIP(k.SrcIPLow, k.SrcIPHigh)
		serverAddr := formatIP(k.DstIPLow, k.DstIPHigh)

		all = append(all, RequestSummary{
			key: key{
				Client: address{
					IP:   clientAddr.String(),
					Port: k.SrcPort,
				},
				Server: address{
					IP:   serverAddr.String(),
					Port: k.DstPort,
				},
			},
		})
	}

	return all
}

func formatIP(low, high uint64) util.Address {
	if high > 0 || (low>>32) > 0 {
		return util.V6Address(low, high)
	}

	return util.V4Address(uint32(low))
}
