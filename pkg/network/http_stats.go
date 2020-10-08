// +build linux_bpf

package network

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
)

type httpStatKeeper struct {
	streams     map[httpStreamKey]*httpStream
	streamStats map[httpStreamKey]httpStreamStats

	// Telemetry
	messagesRead int64
	readErrors   int64
}

type httpStreamStats struct {
	sourceIP   gopacket.Endpoint
	destIP     gopacket.Endpoint
	sourcePort gopacket.Endpoint
	destPort   gopacket.Endpoint

	requests  []httpRequest
	responses []httpResponse

	successes int64
	errors    int64
	// durations []time.Duration

	// lastReqTime time.Time
	// lastResTime time.Time
}

type httpRequest struct {
	method    string
	bodyBytes int
}

type httpResponse struct {
	status    string
	bodyBytes int
}

// PrintConnectionsAndStats is a temporary debug function
func PrintConnectionsAndStats(conns map[httpStreamKey]httpStreamStats, stats map[string]int64) {
	log.Infof("%v HTTP active connections: ", len(conns))
	for _, conn := range conns {
		log.Infof("  %v:%v -> %v:%v \t %v requests, %v responses, %v errors, %v successes ",
			conn.sourceIP, conn.sourcePort, conn.destIP, conn.destPort, len(conn.requests), len(conn.responses), conn.errors, conn.successes)
	}

	// log.Infof("HTTP Telemetry:")
	// for key, val := range stats {
	// 	log.Infof("  %v, %v", key, val)
	// }
}
