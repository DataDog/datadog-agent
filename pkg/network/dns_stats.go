package network

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	key           DNSKey
	pktType       DNSPacketType
	rCode         uint8  // responseCode
	question      string // only relevant for query packets
}

type stateKey struct {
	key DNSKey
	id  uint16
}

type stateValue struct {
	ts       uint64
	question string
}

type dnsStatKeeper struct {
	mux              sync.Mutex
	stats            map[DNSKey]map[string]DNSStats
	state            map[stateKey]stateValue
	expirationPeriod time.Duration
	exit             chan struct{}
	maxSize          int // maximum size of the state map
	deleteCount      int
	numStats         int
	maxStats         int
	droppedStats     int
	lastNumStats     int32
	lastDroppedStats int32
}

func newDNSStatkeeper(timeout time.Duration, maxStats int) *dnsStatKeeper {
	statsKeeper := &dnsStatKeeper{
		stats:            make(map[DNSKey]map[string]DNSStats),
		state:            make(map[stateKey]stateValue),
		expirationPeriod: timeout,
		exit:             make(chan struct{}),
		maxSize:          MaxStateMapSize,
		maxStats:         maxStats,
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
			d.state[sk] = stateValue{question: info.question, ts: microSecs(ts)}
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

	latency := microSecs(ts) - start.ts

	allStats, ok := d.stats[info.key]
	if !ok {
		allStats = make(map[string]DNSStats)
	}
	stats, ok := allStats[start.question]
	if !ok {
		if d.numStats >= d.maxStats {
			d.droppedStats++
			return
		}
		stats.DNSCountByRcode = make(map[uint32]uint32)
		d.numStats++
	}

	// Note: time.Duration in the agent version of go (1.12.9) does not have the Microseconds method.
	if latency > uint64(d.expirationPeriod.Microseconds()) {
		stats.DNSTimeouts++
	} else {
		stats.DNSCountByRcode[uint32(info.rCode)]++
		if info.pktType == SuccessfulResponse {
			stats.DNSSuccessLatencySum += latency
		} else if info.pktType == FailedResponse {
			stats.DNSFailureLatencySum += latency
		}
	}

	allStats[start.question] = stats
	d.stats[info.key] = allStats
}

func (d *dnsStatKeeper) GetNumStats() (int32, int32) {
	numStats := atomic.LoadInt32(&d.lastNumStats)
	droppedStats := atomic.LoadInt32(&d.lastDroppedStats)
	return numStats, droppedStats
}

func (d *dnsStatKeeper) GetAndResetAllStats() map[DNSKey]map[string]DNSStats {
	d.mux.Lock()
	defer d.mux.Unlock()
	ret := d.stats // No deep copy needed since `d.stats` gets reset
	d.stats = make(map[DNSKey]map[string]DNSStats)
	log.Debugf("[DNS Stats] Number of processed stats: %d, Number of dropped stats: %d", d.numStats, d.droppedStats)
	atomic.StoreInt32(&d.lastNumStats, int32(d.numStats))
	atomic.StoreInt32(&d.lastDroppedStats, int32(d.droppedStats))
	d.numStats = 0
	d.droppedStats = 0
	return ret
}

// Snapshot returns a deep copy of all DNS stats.
// Please only use this for testing.
func (d *dnsStatKeeper) Snapshot() map[DNSKey]map[string]DNSStats {
	d.mux.Lock()
	defer d.mux.Unlock()

	snapshot := make(map[DNSKey]map[string]DNSStats)
	for key, statsByDomain := range d.stats {
		snapshot[key] = make(map[string]DNSStats)
		for domain, statsCopy := range statsByDomain {
			// Copy DNSCountByRcode map
			rcodeCopy := make(map[uint32]uint32)
			for rcode, count := range statsCopy.DNSCountByRcode {
				rcodeCopy[rcode] = count
			}

			statsCopy.DNSCountByRcode = rcodeCopy
			snapshot[key][domain] = statsCopy
		}
	}

	return snapshot
}

func (d *dnsStatKeeper) removeExpiredStates(earliestTs time.Time) {
	deleteThreshold := 5000
	d.mux.Lock()
	defer d.mux.Unlock()
	// Any state older than the threshold should be discarded
	threshold := microSecs(earliestTs)
	for k, v := range d.state {
		if v.ts < threshold {
			delete(d.state, k)
			d.deleteCount++
			// When we expire a state, we need to increment timeout count for that key:domain
			allStats, ok := d.stats[k.key]
			if !ok {
				allStats = make(map[string]DNSStats)
			}
			stats, ok := allStats[v.question]
			if !ok {
				if d.numStats >= d.maxStats {
					d.droppedStats++
					continue
				}
				d.numStats++
				stats.DNSCountByRcode = make(map[uint32]uint32)
			}
			stats.DNSTimeouts++
			allStats[v.question] = stats
			d.stats[k.key] = allStats
		}
	}

	if d.deleteCount < deleteThreshold {
		return
	}

	// golang/go#20135 : maps do not shrink after elements removal (delete)
	copied := make(map[stateKey]stateValue, len(d.state))
	for k, v := range d.state {
		copied[k] = v
	}
	d.state = copied
	d.deleteCount = 0
}

func (d *dnsStatKeeper) Close() {
	d.exit <- struct{}{}
}
