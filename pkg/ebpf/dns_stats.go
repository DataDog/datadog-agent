package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"sync"
)

type dnsStats struct {
	lastTransactionID uint16
	// More stats like latency, error, etc. will be added here later
	successfulResponses uint32
}

type dnsKey struct {
	serverIP   util.Address
	clientIP   util.Address
	clientPort uint16
	protocol   ConnectionType
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

func (d *dnsStatKeeper) ProcessSuccessfulResponse(key dnsKey, transactionID uint16) {
	d.mux.Lock()
	defer d.mux.Unlock()
	stats := d.stats[key]

	// For local DNS traffic, sometimes the same reply packet gets processed by the
	// snooper multiple times. This check avoids double counting in that scenario
	// assuming the duplicates are not interleaved.
	if stats.lastTransactionID == transactionID {
		return
	}

	stats.successfulResponses++
	stats.lastTransactionID = transactionID

	d.stats[key] = stats
}

func (d *dnsStatKeeper) GetAndResetAllStats() map[dnsKey]dnsStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	ret := d.stats
	d.stats = make(map[dnsKey]dnsStats)
	return ret
}
