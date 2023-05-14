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
	Client       Address
	Server       Address
	ByRequestAPI map[string]int
	TopicName    string
}

// Address represents represents a IP:Port
type Address struct {
	IP   string
	Port uint16
}

// Stats consolidates request count and latency information for a certain status code
type Stats struct {
	Count int
}

// Kafka returns a debug-friendly representation of map[kafka.Key]kafka.RequestStats
func Kafka(stats map[kafka.Key]*kafka.RequestStat) []RequestSummary {
	all := make([]RequestSummary, 0, len(stats))

	for key, requestStat := range stats {
		clientAddr := formatIP(key.SrcIPLow, key.SrcIPHigh)
		serverAddr := formatIP(key.DstIPLow, key.DstIPHigh)

		byRequestAPI := make(map[string]int)
		switch key.RequestAPIKey {
		case kafka.ProduceAPIKey:
			byRequestAPI["produce"] = requestStat.Count
			break
		case kafka.FetchAPIKey:
			byRequestAPI["fetch"] = requestStat.Count
			break
		}

		debug := RequestSummary{
			Client: Address{
				IP:   clientAddr.String(),
				Port: key.SrcPort,
			},
			Server: Address{
				IP:   serverAddr.String(),
				Port: key.DstPort,
			},

			ByRequestAPI: byRequestAPI,
			TopicName:    key.TopicName,
		}

		all = append(all, debug)
	}

	return all
}

func formatIP(low, high uint64) util.Address {
	// TODO: this is  not correct, but we don't have socket family information
	// for Kafka at the moment, so given this is purely debugging code I think it's fine
	// to assume for now that it's only IPv6 if higher order bits are set.
	if high > 0 || (low>>32) > 0 {
		return util.V6Address(low, high)
	}

	return util.V4Address(uint32(low))
}
