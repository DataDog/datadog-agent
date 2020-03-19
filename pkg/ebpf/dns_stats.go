package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"sync"
)

type dnsStats struct {
	replies uint32
	// More stats like latency, error, etc. will be added here later
}

type dnsData struct {
	stats             dnsStats
	lastTransactionID uint16
}

type dnsKey struct {
	serverIP   util.Address
	clientIP   util.Address
	clientPort uint16
	protocol   ConnectionType
}

type dnsStatKeeper struct {
	mux  sync.Mutex
	data map[dnsKey]dnsData
}

func newDNSStatkeeper() *dnsStatKeeper {
	return &dnsStatKeeper{
		data: make(map[dnsKey]dnsData),
	}
}

func (d *dnsStatKeeper) IncrementReplyCount(key dnsKey, transactionID uint16) {
	d.mux.Lock()
	defer d.mux.Unlock()
	dnsData := d.data[key]

	// For local DNS traffic, sometimes the same reply packet gets processed by the
	// snooper multiple times. This check avoids double counting in that scenario.
	if dnsData.lastTransactionID == transactionID {
		return
	}

	dnsData.stats.replies++
	dnsData.lastTransactionID = transactionID
	d.data[key] = dnsData
}

func (d *dnsStatKeeper) GetAndResetAllStats() map[dnsKey]dnsStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	allStats := make(map[dnsKey]dnsStats, len(d.data))
	for k, v := range d.data {
		allStats[k] = v.stats
	}

	d.data = make(map[dnsKey]dnsData)
	return allStats
}
