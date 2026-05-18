// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"os"
	"strconv"
	"sync"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

const (
	defaultExperimentalIngressLogMaxBytes = 16 * 1024 * 1024
)

func experimentalIngressLogEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG"))
	return err == nil && enabled
}

func experimentalIngressLogMaxBytes() int64 {
	value := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES")
	if value == "" {
		return defaultExperimentalIngressLogMaxBytes
	}
	maxBytes, err := strconv.ParseInt(value, 10, 64)
	if err != nil || maxBytes <= 0 {
		return defaultExperimentalIngressLogMaxBytes
	}
	return maxBytes
}

type packetIngressLogTelemetry struct {
	bytes     telemetry.Gauge
	batches   telemetry.Gauge
	packets   telemetry.Gauge
	blockedNS telemetry.Counter
	stats     telemetry.Counter
}

// packetIngressLog is an experimental bounded in-memory ingress log used to
// replace the large DogStatsD packetsIn channel as the overload absorber. It is
// intentionally byte-bounded so backpressure is expressed as listener append
// blocking instead of unconstrained heap-backed packet buffering.
type packetIngressLog struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	maxBytes int64
	stopped  bool

	batches []packets.Packets
	head    int
	bytes   int64
	packets int64

	telemetry packetIngressLogTelemetry
}

func newPacketIngressLog(maxBytes int64, telemetrycomp telemetry.Component) *packetIngressLog {
	log := &packetIngressLog{maxBytes: maxBytes}
	log.notEmpty = sync.NewCond(&log.mu)
	log.notFull = sync.NewCond(&log.mu)
	if telemetrycomp != nil {
		log.telemetry = packetIngressLogTelemetry{
			bytes: telemetrycomp.NewGauge("dogstatsd_ingress_log", "bytes",
				[]string{}, "Bytes currently retained by the experimental DogStatsD ingress log"),
			batches: telemetrycomp.NewGauge("dogstatsd_ingress_log", "batches",
				[]string{}, "Batches currently retained by the experimental DogStatsD ingress log"),
			packets: telemetrycomp.NewGauge("dogstatsd_ingress_log", "packets",
				[]string{}, "Packets currently retained by the experimental DogStatsD ingress log"),
			blockedNS: telemetrycomp.NewCounter("dogstatsd_ingress_log", "blocked_ns",
				[]string{}, "Nanoseconds spent blocked appending to the experimental DogStatsD ingress log"),
			stats: telemetrycomp.NewCounter("dogstatsd_ingress_log", "stats",
				[]string{"stat"}, "Experimental DogStatsD ingress log counters"),
		}
	}
	return log
}

func (l *packetIngressLog) run(input <-chan packets.Packets, output chan<- packets.Packets, stop <-chan bool) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				l.stop()
				return
			case ps := <-input:
				if !l.append(ps) {
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			ps, ok := l.next()
			if !ok {
				return
			}
			select {
			case <-stop:
				l.stop()
				return
			case output <- ps:
			}
		}
	}()
	wg.Wait()
}

func (l *packetIngressLog) append(ps packets.Packets) bool {
	batchBytes := packetBatchSizeBytes(ps)
	batchPackets := int64(len(ps))
	var blockStart time.Time
	blocked := false

	l.mu.Lock()
	for !l.stopped && l.queueLenLocked() > 0 && l.bytes+batchBytes > l.maxBytes {
		if !blocked {
			blocked = true
			blockStart = time.Now()
		}
		l.notFull.Wait()
	}
	if l.stopped {
		l.mu.Unlock()
		return false
	}
	if blocked && l.telemetry.blockedNS != nil {
		l.telemetry.blockedNS.Add(float64(time.Since(blockStart).Nanoseconds()))
	}
	l.batches = append(l.batches, ps)
	l.bytes += batchBytes
	l.packets += batchPackets
	l.updateGaugesLocked()
	l.notEmpty.Signal()
	l.mu.Unlock()

	if l.telemetry.stats != nil {
		l.telemetry.stats.Inc("appended_batches")
		l.telemetry.stats.Add(float64(batchPackets), "appended_packets")
	}
	return true
}

func (l *packetIngressLog) next() (packets.Packets, bool) {
	l.mu.Lock()
	for !l.stopped && l.queueLenLocked() == 0 {
		l.notEmpty.Wait()
	}
	if l.queueLenLocked() == 0 {
		l.mu.Unlock()
		return nil, false
	}

	ps := l.batches[l.head]
	l.batches[l.head] = nil
	l.head++
	l.bytes -= packetBatchSizeBytes(ps)
	l.packets -= int64(len(ps))
	if l.bytes < 0 {
		l.bytes = 0
	}
	if l.packets < 0 {
		l.packets = 0
	}
	l.compactLocked()
	l.updateGaugesLocked()
	l.notFull.Signal()
	l.mu.Unlock()

	if l.telemetry.stats != nil {
		l.telemetry.stats.Inc("taken_batches")
		l.telemetry.stats.Add(float64(len(ps)), "taken_packets")
	}
	return ps, true
}

func (l *packetIngressLog) stop() {
	l.mu.Lock()
	l.stopped = true
	l.notEmpty.Broadcast()
	l.notFull.Broadcast()
	l.mu.Unlock()
}

func (l *packetIngressLog) queueLenLocked() int {
	return len(l.batches) - l.head
}

func (l *packetIngressLog) compactLocked() {
	if l.head == 0 {
		return
	}
	if l.head < 1024 && l.head*2 < len(l.batches) {
		return
	}
	copy(l.batches, l.batches[l.head:])
	remaining := len(l.batches) - l.head
	for i := remaining; i < len(l.batches); i++ {
		l.batches[i] = nil
	}
	l.batches = l.batches[:remaining]
	l.head = 0
}

func (l *packetIngressLog) updateGaugesLocked() {
	if l.telemetry.bytes != nil {
		l.telemetry.bytes.Set(float64(l.bytes))
	}
	if l.telemetry.batches != nil {
		l.telemetry.batches.Set(float64(l.queueLenLocked()))
	}
	if l.telemetry.packets != nil {
		l.telemetry.packets.Set(float64(l.packets))
	}
}

func packetBatchSizeBytes(ps packets.Packets) int64 {
	return int64((&ps).SizeInBytes() + (&ps).DataSizeInBytes())
}
