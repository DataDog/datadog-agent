// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package debugging provides debug-friendly representations of internal data structures
package debugging

import (
	"github.com/DataDog/datadog-agent/pkg/network"
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
func Kafka(cs *network.Connections) []RequestSummary {
	var all []RequestSummary

	for _, c := range cs.Conns {
		for _, s := range c.KafkaStats {
			clientAddr := formatIP(s.Key.SrcIPLow, s.Key.SrcIPHigh)
			serverAddr := formatIP(s.Key.DstIPLow, s.Key.DstIPHigh)

			byRequestAPI := make(map[string]int)
			switch s.Key.RequestAPIKey {
			case kafka.ProduceAPIKey:
				byRequestAPI["produce"] = s.Value.Count
			case kafka.FetchAPIKey:
				byRequestAPI["fetch"] = s.Value.Count
			}

			debug := RequestSummary{
				Client: Address{
					IP:   clientAddr.String(),
					Port: s.Key.SrcPort,
				},
				Server: Address{
					IP:   serverAddr.String(),
					Port: s.Key.DstPort,
				},

				ByRequestAPI: byRequestAPI,
				TopicName:    s.Key.TopicName,
			}

			all = append(all, debug)
		}
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
