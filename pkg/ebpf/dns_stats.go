package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"sync"
)

type info struct {
	replies int64
	lastTransactionID uint16
}

type connKey struct {
	serverIP  util.Address
	queryIP   util.Address
	queryPort int32
	protocol  ConnectionType
}

type dnsStats struct {
	mux  sync.Mutex
	metrics map[connKey]info
}

func newDNSStats() *dnsStats {
	return &dnsStats{
		metrics: make(map[connKey]info),
	}
}

func (d *dnsStats) IncrementReplyCount(key connKey, transactionID uint16) {
	d.mux.Lock()
	defer d.mux.Unlock()
}
