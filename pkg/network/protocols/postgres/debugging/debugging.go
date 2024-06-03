// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package debugging provides debug-friendly representations of internal data structures
package debugging

import (
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// address represents represents a IP:Port
type address struct {
	IP   string
	Port uint16
}

// key represents a (client, server, table name) tuple.
type key struct {
	Client    address
	Server    address
	TableName string
}

// Stats consolidates request count and latency information for a certain status code
type Stats struct {
	Count              int
	FirstLatencySample float64
	LatencyP50         float64
	latencies          *ddsketch.DDSketch
}

// RequestSummary represents a (debug-friendly) aggregated view of requests
// matching a (client, server, table name, operation) tuple
type RequestSummary struct {
	key
	ByOperation map[string]Stats
}

// Postgres returns a debug-friendly representation of map[postgres.Key]postgres.RequestStats
func Postgres(stats map[postgres.Key]*postgres.RequestStat) []RequestSummary {
	resMap := make(map[key]map[string]Stats)
	for k, requestStat := range stats {
		clientAddr := formatIP(k.SrcIPLow, k.SrcIPHigh)
		serverAddr := formatIP(k.DstIPLow, k.DstIPHigh)

		tempKey := key{
			Client: address{
				IP:   clientAddr.String(),
				Port: k.SrcPort,
			},
			Server: address{
				IP:   serverAddr.String(),
				Port: k.DstPort,
			},
			TableName: k.TableName,
		}
		if _, ok := resMap[tempKey]; !ok {
			resMap[tempKey] = make(map[string]Stats)
		}
		if _, ok := resMap[tempKey][k.Operation.String()]; !ok {
			resMap[tempKey][k.Operation.String()] = Stats{}
		}
		currentStats := resMap[tempKey][k.Operation.String()]
		currentStats.Count += requestStat.Count
		if currentStats.FirstLatencySample == 0 {
			currentStats.FirstLatencySample = requestStat.FirstLatencySample
		}
		if requestStat.Latencies != nil {
			if currentStats.latencies == nil {
				currentStats.latencies = requestStat.Latencies.Copy()
			} else {
				if err := currentStats.latencies.MergeWith(requestStat.Latencies); err != nil {
					log.Debugf("could not add request latency to ddsketch: %v", err)
				}
			}
		}

		resMap[tempKey][k.Operation.String()] = currentStats
	}

	all := make([]RequestSummary, 0, len(resMap))
	for key, value := range resMap {
		for operation, stats := range value {
			stats.LatencyP50 = getSketchQuantile(stats.latencies, 0.5)
			value[operation] = stats
		}
		debug := RequestSummary{
			key:         key,
			ByOperation: value,
		}
		all = append(all, debug)
	}
	return all
}

func formatIP(low, high uint64) util.Address {
	if high > 0 || (low>>32) > 0 {
		return util.V6Address(low, high)
	}

	return util.V4Address(uint32(low))
}

func getSketchQuantile(sketch *ddsketch.DDSketch, percentile float64) float64 {
	if sketch == nil {
		return 0.0
	}

	val, _ := sketch.GetValueAtQuantile(percentile)
	return val
}
