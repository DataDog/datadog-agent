// +build linux_bpf

package network

import (
	"net/url"
	"sync"
	"time"

	"github.com/google/gopacket"
)

type packetDirection uint8

const (
	request = iota
	response
	unknown
)

type httpStatKeeper struct {
	mux         sync.Mutex
	connections map[httpKey]httpConnection

	// Telemetry
	messagesRead int64
	readErrors   int64
}

type httpKey struct {
	sourceIP   gopacket.Endpoint
	destIP     gopacket.Endpoint
	sourcePort gopacket.Endpoint
	destPort   gopacket.Endpoint
}

func newKey(packet gopacket.Packet) httpKey {
	srcIP, dstIP := packet.NetworkLayer().NetworkFlow().Endpoints()
	srcPort, dstPort := packet.TransportLayer().TransportFlow().Endpoints()

	return httpKey{
		sourceIP:   srcIP,
		destIP:     dstIP,
		sourcePort: srcPort,
		destPort:   dstPort,
	}
}

type httpConnection struct {
	requests  []httpRequest
	responses []httpResponse

	lastReqTime time.Time
	lastResTime time.Time
}

type httpRequest struct {
	method    string
	url       *url.URL
	bodyBytes int
}

type httpResponse struct {
	status string
}
