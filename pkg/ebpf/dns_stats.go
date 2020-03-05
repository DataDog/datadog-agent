package ebpf

import (
	"fmt"
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
	queryPort uint16
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
	dnsInfo := d.metrics[key]
	dnsInfo.replies++
	d.metrics[key] = dnsInfo
	fmt.Println("Incremented")
	fmt.Println(key)
}

func (d *dnsStats) Get(key connKey) info {
	d.mux.Lock()
	defer d.mux.Unlock()
	fmt.Println(key)
	fmt.Println(d.metrics[key])
	return d.metrics[key]
}
