// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package network

import "github.com/DataDog/datadog-agent/pkg/network/protocols/http"

// storeUSMStats is a generic function to store USM stats for all clients.
func storeUSMStats[K, S comparable](
	allStats map[K]S,
	clients map[string]*client,
	getDelta func(*client) map[K]S,
	setDelta func(*client, map[K]S),
	combineStats func(S, S),
	limit int,
	incDropped func(...string),
) {
	if len(clients) == 1 {
		for _, client := range clients {
			delta := getDelta(client)
			if len(delta) == 0 && len(allStats) <= limit {
				setDelta(client, allStats)
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range clients {
			delta := getDelta(client)
			prevStats, ok := delta[key]
			if !ok && len(delta) >= limit {
				incDropped()
				continue
			}

			var zero S
			if prevStats != zero {
				combineStats(prevStats, stats)
				delta[key] = prevStats
			} else {
				delta[key] = stats
			}
		}
	}
}

// storeHTTPStats stores the latest HTTP stats for all clients
func (ns *networkState) storeHTTPStats(allStats map[http.Key]*http.RequestStats) {
	storeUSMStats[http.Key, *http.RequestStats](
		allStats,
		ns.clients,
		func(c *client) map[http.Key]*http.RequestStats { return c.usmDelta.HTTP },
		func(c *client, m map[http.Key]*http.RequestStats) { c.usmDelta.HTTP = m },
		func(prev, new *http.RequestStats) { prev.CombineWith(new) },
		ns.maxHTTPStats,
		stateTelemetry.httpStatsDropped.Inc,
	)
}
