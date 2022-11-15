// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debugging

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// RequestSummary represents a (debug-friendly) aggregated view of requests
// matching a (client, server, path, method) tuple
type RequestSummary struct {
	Client Address
	Server Address
	//DNS         string
	//Path        string
	//Method      string
	//ByStatus    map[int]Stats
	ByRequestAPI map[string]int
	TopicName    string
	//StaticTags  uint64
	//DynamicTags []string
}

// Address represents represents a IP:Port
type Address struct {
	IP   string
	Port uint16
}

// Stats consolidates request count and latency information for a certain status code
type Stats struct {
	Count int
	//FirstLatencySample float64
	//LatencyP50         float64
}

// Kafka returns a debug-friendly representation of map[kafka.Key]kafka.RequestStats
func Kafka(stats map[kafka.Key]*kafka.RequestStats) []RequestSummary {
	all := make([]RequestSummary, 0, len(stats))
	for key, requestStats := range stats {
		clientAddr := formatIP(key.SrcIPLow, key.SrcIPHigh)
		serverAddr := formatIP(key.DstIPLow, key.DstIPHigh)

		byRequestAPI := make(map[string]int)
		byRequestAPI["produce"] = requestStats.Data[0].Count
		byRequestAPI["fetch"] = requestStats.Data[1].Count

		debug := RequestSummary{
			Client: Address{
				IP:   clientAddr.String(),
				Port: key.SrcPort,
			},
			Server: Address{
				IP:   serverAddr.String(),
				Port: key.DstPort,
			},

			//DNS:      getDNS(dns, serverAddr),
			//Path:     k.Path.Content,
			//Method:   k.Method.String(),
			ByRequestAPI: byRequestAPI,
			TopicName:    key.TopicName,
		}

		//for status := 100; status <= 500; status += 100 {
		//	if !v.HasStats(status) {
		//		continue
		//	}
		//	stat := v.Stats(status)
		//	debug.StaticTags = stat.StaticTags
		//	debug.DynamicTags = stat.DynamicTags
		//
		//	debug.ByStatus[status] = Stats{
		//		Count:              stat.Count,
		//		FirstLatencySample: stat.FirstLatencySample,
		//		LatencyP50:         getSketchQuantile(stat.Latencies, 0.5),
		//	}
		//}

		all = append(all, debug)
	}

	return all
}

func formatIP(low, high uint64) util.Address {
	// TODO: this is  not correct, but we don't have socket family information
	// for HTTP at the moment, so given this is purely debugging code I think it's fine
	// to assume for now that it's only IPv6 if higher order bits are set.
	if high > 0 || (low>>32) > 0 {
		return util.V6Address(low, high)
	}

	return util.V4Address(uint32(low))
}
