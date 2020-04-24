package ebpf

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type dnsStats struct {
	lastTransactionID uint16
	// More stats like latency, error, etc. will be added here later
	successfulResponses uint32
	failedResponses     uint32
}

type dnsKey struct {
	serverIP   util.Address
	clientIP   util.Address
	clientPort uint16
	// ConnectionType will be either TCP or UDP
	protocol ConnectionType
}

// DNSPacketType tells us whether the packet is a query or a reply (successful/failed)
type DNSPacketType uint8

const (
	// SuccessfulResponse indicates that the response code of the DNS reply is 0
	SuccessfulResponse DNSPacketType = iota
	// FailedResponse indicates that the response code of the DNS reply is anything other than 0
	FailedResponse
)

type dnsPacketInfo struct {
	transactionID uint16
	key           dnsKey
	pktType       DNSPacketType
}

type dnsStatKeeper struct {
	mux   sync.Mutex
	stats map[dnsKey]dnsStats
}

func newDNSStatkeeper() *dnsStatKeeper {
	return &dnsStatKeeper{
		stats: make(map[dnsKey]dnsStats),
	}
}

func (d *dnsStatKeeper) ProcessPacketInfo(info dnsPacketInfo) {
	d.mux.Lock()
	defer d.mux.Unlock()
	stats := d.stats[info.key]

	// For local DNS traffic, sometimes the same reply packet gets processed by the
	// snooper multiple times. This check avoids double counting in that scenario
	// assuming the duplicates are not interleaved.
	if stats.lastTransactionID == info.transactionID {
		return
	}

	if info.pktType == SuccessfulResponse {
		stats.successfulResponses++
	} else {
		stats.failedResponses++
	}

	stats.lastTransactionID = info.transactionID

	d.stats[info.key] = stats
}

func (d *dnsStatKeeper) GetAndResetAllStats() map[dnsKey]dnsStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	ret := d.stats
	d.stats = make(map[dnsKey]dnsStats)
	return ret
}
