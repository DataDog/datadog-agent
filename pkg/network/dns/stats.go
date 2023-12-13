// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf

package dns

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// packetType tells us whether the packet is a query or a reply (successful/failed)
type packetType uint8

const (
	// successfulResponse means the packet contains a DNS response and the response code is 0 (no error)
	successfulResponse packetType = iota
	// failedResponse means the packet contains a DNS response and the response code is not 0
	failedResponse
	// query means the packet contains a DNS query
	query
	// Subsystem name for telemetry purposes
	dnsStatKeeperModuleName = "network_tracer__dns_stat_keeper"
	// This const limits the maximum size of the state map. Benchmark results show that allocated space is less than 3MB
	// for 10000 entries.
	maxStateMapSize = 10000
)

var statsTelemetry = struct {
	processedStats telemetry.Counter
	droppedStats   telemetry.Counter
}{
	telemetry.NewCounter(dnsStatKeeperModuleName, "processed_stats", []string{}, "Counter measuring the number of processed DNS stats"),
	telemetry.NewCounter(dnsStatKeeperModuleName, "dropped_stats", []string{}, "Counter measuring the number of dropped DNS stats"),
}

type dnsPacketInfo struct {
	transactionID uint16
	key           Key
	pktType       packetType
	rCode         uint8    // responseCode
	question      Hostname // only relevant for query packets
	queryType     QueryType
}

type stateKey struct {
	key Key
	id  uint16
}

type stateValue struct {
	ts       uint64
	question Hostname
	qtype    QueryType
}

type dnsStatKeeper struct {
	mux sync.Mutex
	// map a DNS key to a map of domain strings to a map of query types to a map of  DNS stats
	stats            StatsByKeyByNameByType
	state            map[stateKey]stateValue
	expirationPeriod time.Duration
	exit             chan struct{}
	maxSize          int // maximum size of the state map
	deleteCount      int
	processedStats   int64
	droppedStats     int64
	maxStats         int64
}

func newDNSStatkeeper(timeout time.Duration, maxStats int64) *dnsStatKeeper {
	statsKeeper := &dnsStatKeeper{
		stats:            make(StatsByKeyByNameByType),
		state:            make(map[stateKey]stateValue),
		expirationPeriod: timeout,
		exit:             make(chan struct{}),
		maxSize:          maxStateMapSize,
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

	if info.pktType == query {
		if len(d.state) == d.maxSize {
			return
		}

		if _, ok := d.state[sk]; !ok {
			d.state[sk] = stateValue{question: info.question, ts: microSecs(ts), qtype: info.queryType}
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
		allStats = make(map[Hostname]map[QueryType]Stats)
	}
	stats, ok := allStats[start.question]
	if !ok {
		if d.processedStats >= d.maxStats {
			d.droppedStats++
			statsTelemetry.droppedStats.Inc()
			return
		}
		stats = make(map[QueryType]Stats)
	}
	byqtype, ok := stats[start.qtype]
	if !ok {
		if d.processedStats >= d.maxStats {
			d.droppedStats++
			statsTelemetry.droppedStats.Inc()
			return
		}
		byqtype.CountByRcode = make(map[uint32]uint32)
		d.processedStats++
		statsTelemetry.processedStats.Inc()
	}

	// Note: time.Duration in the agent version of go (1.12.9) does not have the Microseconds method.
	if latency > uint64(d.expirationPeriod.Microseconds()) {
		byqtype.Timeouts++
	} else {
		byqtype.CountByRcode[uint32(info.rCode)]++
		if info.pktType == successfulResponse {
			byqtype.SuccessLatencySum += latency
		} else if info.pktType == failedResponse {
			byqtype.FailureLatencySum += latency
		}
	}
	stats[start.qtype] = byqtype
	allStats[start.question] = stats
	d.stats[info.key] = allStats
}

func (d *dnsStatKeeper) GetAndResetAllStats() StatsByKeyByNameByType {
	d.mux.Lock()
	defer d.mux.Unlock()
	ret := d.stats // No deep copy needed since `d.stats` gets reset
	d.stats = make(StatsByKeyByNameByType)
	log.Debugf("[DNS Stats] Number of processed stats: %d, Number of dropped stats: %d", d.processedStats, d.droppedStats)
	d.processedStats = 0
	d.droppedStats = 0
	return ret
}

// Snapshot returns a deep copy of all DNS stats.
// Please only use this for testing.
func (d *dnsStatKeeper) Snapshot() StatsByKeyByNameByType {
	d.mux.Lock()
	defer d.mux.Unlock()

	snapshot := make(StatsByKeyByNameByType)
	for key, statsByDomain := range d.stats {
		snapshot[key] = make(map[Hostname]map[QueryType]Stats)
		for domain, statsByQType := range statsByDomain {
			snapshot[key][domain] = make(map[QueryType]Stats)
			for qtype, statsCopy := range statsByQType {
				// Copy CountByRcode map
				rcodeCopy := make(map[uint32]uint32)
				for rcode, count := range statsCopy.CountByRcode {
					rcodeCopy[rcode] = count
				}
				statsCopy.CountByRcode = rcodeCopy
				snapshot[key][domain][qtype] = statsCopy
			}
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
				allStats = make(map[Hostname]map[QueryType]Stats)
			}
			bytype, ok := allStats[v.question]
			if !ok {
				if d.processedStats >= d.maxStats {
					d.droppedStats++
					statsTelemetry.droppedStats.Inc()
					continue
				}
				bytype = make(map[QueryType]Stats)
			}
			stats, ok := bytype[v.qtype]
			if !ok {
				d.processedStats++
				statsTelemetry.processedStats.Inc()
				stats.CountByRcode = make(map[uint32]uint32)
			}
			stats.Timeouts++
			bytype[v.qtype] = stats
			allStats[v.question] = bytype
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
	close(d.exit)
}
