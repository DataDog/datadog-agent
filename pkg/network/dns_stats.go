package network

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type dnsStats struct {
	// More stats like latency, error, etc. will be added here later
	successfulResponses uint32
	failedResponses     uint32
	successLatencySum   uint64 // Stored in µs
	failureLatencySum   uint64
	timeouts            uint32
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
	// SuccessfulResponse means the packet contains a DNS response and the response code is 0 (no error)
	SuccessfulResponse DNSPacketType = iota
	// FailedResponse means the packet contains a DNS response and the response code is not 0
	FailedResponse
	// Query means the packet contains a DNS query
	Query
)

// This const limits the maximum size of the state map. Benchmark results show that allocated space is less than 3MB
// for 10000 entries.
const (
	MaxStateMapSize = 10000
)

type dnsPacketInfo struct {
	transactionID uint16
	key           dnsKey
	pktType       DNSPacketType
}

type stateKey struct {
	key dnsKey
	id  uint16
}

type dnsStatKeeper struct {
	mux              sync.Mutex
	stats            map[dnsKey]dnsStats
	state            map[stateKey]uint64
	expirationPeriod time.Duration
	exit             chan struct{}
	maxSize          int // maximum size of the state map
	deleteCount      int
}

func newDNSStatkeeper(timeout time.Duration) *dnsStatKeeper {
	statsKeeper := &dnsStatKeeper{
		stats:            make(map[dnsKey]dnsStats),
		state:            make(map[stateKey]uint64),
		expirationPeriod: timeout,
		exit:             make(chan struct{}),
		maxSize:          MaxStateMapSize,
	}

	ticker := time.NewTicker(statsKeeper.expirationPeriod)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				statsKeeper.removeExpiredStates(now.Add(-statsKeeper.expirationPeriod))
			case <-statsKeeper.exit:
				ticker.Stop()
				return
			}
		}
	}()
	return statsKeeper
}

func microSecs(t time.Time) uint64 {
	return uint64(t.UnixNano() / 1000)
}

func (d *dnsStatKeeper) ProcessPacketInfo(info dnsPacketInfo, ts time.Time) {
	d.mux.Lock()
	defer d.mux.Unlock()
	sk := stateKey{key: info.key, id: info.transactionID}

	if info.pktType == Query {
		if len(d.state) == d.maxSize {
			return
		}

		if _, ok := d.state[sk]; !ok {
			d.state[sk] = microSecs(ts)
		}
		return
	}

	// If a response does not have a corresponding query entry, we discard it
	start, ok := d.state[sk]

	if !ok {
		return
	}

	delete(d.state, sk)
	d.deleteCount++

	latency := microSecs(ts) - start

	stats := d.stats[info.key]

	// Note: time.Duration in the agent version of go (1.12.9) does not have the Microseconds method.
	if latency > uint64(d.expirationPeriod.Microseconds()) {
		stats.timeouts++
	} else {
		if info.pktType == SuccessfulResponse {
			stats.successfulResponses++
			stats.successLatencySum += latency
		} else if info.pktType == FailedResponse {
			stats.failedResponses++
			stats.failureLatencySum += latency
		}
	}

	d.stats[info.key] = stats
}

func (d *dnsStatKeeper) GetAndResetAllStats() map[dnsKey]dnsStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	ret := d.stats
	d.stats = make(map[dnsKey]dnsStats)
	return ret
}

func (d *dnsStatKeeper) removeExpiredStates(earliestTs time.Time) {
	deleteThreshold := 5000
	d.mux.Lock()
	defer d.mux.Unlock()
	threshold := microSecs(earliestTs)
	for k, v := range d.state {
		if v < threshold {
			delete(d.state, k)
			d.deleteCount++
			stats := d.stats[k.key]
			stats.timeouts++
			d.stats[k.key] = stats
		}
	}

	if d.deleteCount < deleteThreshold {
		return
	}

	// golang/go#20135 : maps do not shrink after elements removal (delete)
	copied := make(map[stateKey]uint64, len(d.state))
	for k, v := range d.state {
		copied[k] = v
	}
	d.state = copied
	d.deleteCount = 0
}

func (d *dnsStatKeeper) Close() {
	d.exit <- struct{}{}
}
