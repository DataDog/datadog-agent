package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"sync"
)

type dnsStats struct {
	replies           uint32
	lastTransactionID uint16
}

type connKey struct {
	serverIP  util.Address
	queryIP   util.Address
	queryPort uint16
	protocol  ConnectionType
}

type dnsBookkeeper struct {
	mux   sync.Mutex
	stats map[connKey]dnsStats
}

func newDNSBookkeeper() *dnsBookkeeper {
	return &dnsBookkeeper{
		stats: make(map[connKey]dnsStats),
	}
}

func (d *dnsBookkeeper) IncrementReplyCount(key connKey, transactionID uint16) {
	d.mux.Lock()
	defer d.mux.Unlock()
	stats := d.stats[key]
	if stats.lastTransactionID == transactionID {
		return
	}
	stats.replies++
	stats.lastTransactionID = transactionID
	d.stats[key] = stats
}

func (d *dnsBookkeeper) Get(key connKey) dnsStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	return d.stats[key]
}
