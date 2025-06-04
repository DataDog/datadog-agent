// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package debugging provides debug-friendly representation of internal data structures
package debugging

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
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

// Stats consolidates request count and latency information for a certain RequestSummary ErrorToStats entry.
type Stats struct {
	Count              int
	FirstLatencySample float64
	LatencyP50         float64
}

// RequestSummary represents a (debug-friendly) aggregated view of requests
type RequestSummary struct {
	key
	Command      string
	KeyName      string
	Truncated    bool
	ErrorToStats map[int32]Stats
}

// Redis returns a debug-friendly representation of map[redis.Key]redis.RequestStats
func Redis(stats map[redis.Key]*redis.RequestStats) []RequestSummary {
	var requestCount int
	for k := range stats {
		requestCount += len(stats[k].ErrorToStats)
	}
	all := make([]RequestSummary, 0, requestCount)

	for k := range stats {
		clientAddr := formatIP(k.SrcIPLow, k.SrcIPHigh)
		serverAddr := formatIP(k.DstIPLow, k.DstIPHigh)

		requestSummary := RequestSummary{
			Truncated: k.Truncated,
			KeyName:   k.KeyName.Get(),
			Command:   k.Command.String(),
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
			ErrorToStats: make(map[int32]Stats, len(stats[k].ErrorToStats)),
		}

		for err, stat := range stats[k].ErrorToStats {
			errKey := int32(model.RedisErrorType_RedisNoError)
			if err {
				errKey = int32(model.RedisErrorType_RedisErrorTypeUnknown)
			}
			requestSummary.ErrorToStats[errKey] = Stats{
				Count:              stat.Count,
				FirstLatencySample: stat.FirstLatencySample,
				LatencyP50:         protocols.GetSketchQuantile(stat.Latencies, 0.5),
			}
		}
		all = append(all, requestSummary)
	}

	return all
}

func formatIP(low, high uint64) util.Address {
	if high > 0 || (low>>32) > 0 {
		return util.V6Address(low, high)
	}

	return util.V4Address(uint32(low))
}
