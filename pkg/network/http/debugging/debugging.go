// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debugging

import (
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// RequestSummary represents a (debug-friendly) aggregated view of requests
// matching a (client, server, path, method) tuple
type RequestSummary struct {
	Client      Address
	Server      Address
	DNS         string
	Path        string
	Method      string
	ByStatus    map[int]Stats
	StaticTags  uint64
	DynamicTags []string
}

// Address represents represents a IP:Port
type Address struct {
	IP   string
	Port uint16
}

// Stats consolidates request count and latency information for a certain status code
type Stats struct {
	Count              int
	FirstLatencySample float64
	LatencyP50         float64
}

// HTTP returns a debug-friendly representation of map[http.Key]http.RequestStats
func HTTP(stats map[transaction.Key]*http.RequestStats, dns map[util.Address][]dns.Hostname) []RequestSummary {
	all := make([]RequestSummary, 0, len(stats))
	for k, v := range stats {
		clientAddr := formatIP(k.SrcIPLow, k.SrcIPHigh)
		serverAddr := formatIP(k.DstIPLow, k.DstIPHigh)

		debug := RequestSummary{
			Client: Address{
				IP:   clientAddr.String(),
				Port: k.SrcPort,
			},
			Server: Address{
				IP:   serverAddr.String(),
				Port: k.DstPort,
			},
			DNS:      getDNS(dns, serverAddr),
			Path:     k.Path.Content,
			Method:   k.Method.String(),
			ByStatus: make(map[int]Stats),
		}

		for status := 100; status <= 500; status += 100 {
			if !v.HasStats(status) {
				continue
			}
			stat := v.Stats(status)
			debug.StaticTags = stat.StaticTags
			debug.DynamicTags = stat.DynamicTags

			debug.ByStatus[status] = Stats{
				Count:              stat.Count,
				FirstLatencySample: stat.FirstLatencySample,
				LatencyP50:         getSketchQuantile(stat.Latencies, 0.5),
			}
		}

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

func getDNS(dnsData map[util.Address][]dns.Hostname, addr util.Address) string {
	if names := dnsData[addr]; len(names) > 0 {
		return dns.ToString(names[0])
	}

	return ""
}

func getSketchQuantile(sketch *ddsketch.DDSketch, percentile float64) float64 {
	if sketch == nil {
		return 0.0
	}

	val, _ := sketch.GetValueAtQuantile(percentile)
	return val
}
