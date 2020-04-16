package ebpf

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type dnsStats struct {
	// More stats like latency, error, etc. will be added here later
	successfulResponses uint32
	failedResponses     uint32
	latency             float64
	timeouts            uint32
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
	SuccessfulResponse DNSPacketType = 0
	FailedResponse     DNSPacketType = 1
	Query              DNSPacketType = 2
)

type dnsPacketInfo struct {
	transactionID uint16
	key           dnsKey
	type_         DNSPacketType
}

type stateKey struct {
	key dnsKey
	id  uint16
}

type dnsStatKeeper struct {
	mux              sync.Mutex
	stats            map[dnsKey]dnsStats
	state            map[stateKey]time.Time
	expirationPeriod time.Duration
	exit             chan struct{}
}

func newDNSStatkeeper() *dnsStatKeeper {
	statsKeeper := &dnsStatKeeper{
		stats:            make(map[dnsKey]dnsStats),
		expirationPeriod: 30 * time.Second, // TODO: make it configurable
		exit:             make(chan struct{}),
	}

	ticker := time.NewTicker(statsKeeper.expirationPeriod)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				statsKeeper.removeExpiredStates(now.Add(-statsKeeper.expirationPeriod))
			case <-statsKeeper.exit:
				return
			}
		}
	}()
	return statsKeeper
}

func (d *dnsStatKeeper) ProcessPacketInfo(info dnsPacketInfo, ts time.Time) {
	d.mux.Lock()
	defer d.mux.Unlock()
	sk := stateKey{key: info.key, id: info.transactionID}

	if info.type_ == Query {
		if _, ok := d.state[sk]; !ok {
			d.state[sk] = ts
		}
		return
	}

	// If a response does not have a corresponding query entry, we discard it
	start, ok := d.state[sk]

	if !ok {
		return
	}

	delete(d.state, sk)

	latency := ts.Sub(start).Seconds()

	stats := d.stats[info.key]

	if latency > d.expirationPeriod.Seconds() {
		stats.timeouts++
	} else {
		if info.type_ == SuccessfulResponse {
			stats.successfulResponses++
		} else if info.type_ == FailedResponse {
			stats.failedResponses++
		}
		stats.latency += latency // Need to discuss, should we calculate latency for both successful and failed responses
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
	d.mux.Lock()
	defer d.mux.Unlock()
	for k, v := range d.state {
		if v.Before(earliestTs) {
			delete(d.state, k)
			stats := d.stats[k.key]
			stats.timeouts++
			d.stats[k.key] = stats
		}
	}
}

func (d *dnsStatKeeper) Close() {
	d.exit <- struct{}{}
}
