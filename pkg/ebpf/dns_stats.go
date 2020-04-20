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
	protocol   ConnectionType
}

// ConnectionType will be either TCP or UDP
type DNSPacketType uint8

const (
	SuccessfulResponse DNSPacketType = iota
	FailedResponse
)

type dnsPacketInfo struct {
	transactionID uint16
	key           dnsKey
	type_         DNSPacketType
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

	if info.type_ == SuccessfulResponse {
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
