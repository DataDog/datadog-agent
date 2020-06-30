// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package writer

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// pathStats is the target host API path for delivering stats.
const pathStats = "/api/v0.2/stats"

const (
	// bytesPerEntry specifies the approximate size an entry in a stat payload occupies.
	bytesPerEntry = 125
	// maxEntriesPerPayload is the maximum number of entries in a stat payload. An
	// entry has an average size of 125 bytes in a compressed payload. The current
	// Datadog intake API limits a compressed payload to ~3MB (24,000 entries), but
	// let's have the default ensure we don't have paylods > 1.5 MB (12,000
	// entries).
	maxEntriesPerPayload = 12000
)

// StatsWriter ingests stats buckets and flushes them to the API.
type StatsWriter struct {
	in       <-chan []stats.Bucket
	hostname string
	env      string
	senders  []*sender
	stop     chan struct{}
	stats    *info.StatsWriterInfo

	easylog *logutil.ThrottledLogger
}

// NewStatsWriter returns a new StatsWriter. It must be started using Run.
func NewStatsWriter(cfg *config.AgentConfig, in <-chan []stats.Bucket) *StatsWriter {
	sw := &StatsWriter{
		in:       in,
		hostname: cfg.Hostname,
		env:      cfg.DefaultEnv,
		stats:    &info.StatsWriterInfo{},
		stop:     make(chan struct{}),
		easylog:  logutil.NewThrottled(5, 10*time.Second), // no more than 5 messages every 10 seconds
	}
	climit := cfg.StatsWriter.ConnectionLimit
	if climit == 0 {
		// allow 1% of the connection limit to outgoing sends.
		climit = int(math.Max(1, float64(cfg.ConnectionLimit)/100))
	}
	qsize := cfg.StatsWriter.QueueSize
	if qsize == 0 {
		payloadSize := float64(maxEntriesPerPayload * bytesPerEntry)
		// default to 25% of maximum memory.
		maxmem := cfg.MaxMemory / 4
		if maxmem == 0 {
			// or 250MB if unbound
			maxmem = 250 * 1024 * 1024
		}
		qsize = int(math.Max(1, maxmem/payloadSize))
	}
	log.Debugf("Stats writer initialized (climit=%d qsize=%d)", climit, qsize)
	sw.senders = newSenders(cfg, sw, pathStats, climit, qsize)
	return sw
}

// Run starts the StatsWriter, making it ready to receive stats and report metrics.
func (w *StatsWriter) Run() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	defer close(w.stop)
	for {
		select {
		case stats := <-w.in:
			w.addStats(stats)
		case <-t.C:
			w.report()
		case <-w.stop:
			return
		}
	}
}

// Stop stops a running StatsWriter.
func (w *StatsWriter) Stop() {
	w.stop <- struct{}{}
	<-w.stop
	stopSenders(w.senders)
}

func (w *StatsWriter) addStats(s []stats.Bucket) {
	defer timing.Since("datadog.trace_agent.stats_writer.encode_ms", time.Now())

	payloads, bucketCount, entryCount := w.buildPayloads(s, maxEntriesPerPayload)
	switch n := len(payloads); {
	case n == 0:
		return
	case n > 1:
		atomic.AddInt64(&w.stats.Splits, 1)
	}
	atomic.AddInt64(&w.stats.StatsBuckets, int64(bucketCount))
	log.Debugf("Flushing %d entries (buckets=%d payloads=%v)", entryCount, bucketCount, len(payloads))

	for _, p := range payloads {
		req := newPayload(map[string]string{
			headerLanguages:    strings.Join(info.Languages(), "|"),
			"Content-Type":     "application/json",
			"Content-Encoding": "gzip",
		})
		if err := stats.EncodePayload(req.body, p); err != nil {
			log.Errorf("Stats encoding error: %v", err)
			return
		}
		atomic.AddInt64(&w.stats.Bytes, int64(req.body.Len()))

		sendPayloads(w.senders, req)
	}
}

// buildPayloads returns a set of payload to send out, each paylods guaranteed
// to have the number of stats buckets under the given maximum.
func (w *StatsWriter) buildPayloads(s []stats.Bucket, maxEntriesPerPayloads int) ([]*stats.Payload, int, int) {
	if len(s) == 0 {
		return []*stats.Payload{}, 0, 0
	}
	// 1. Get an estimate of how many payloads we need, based on the total
	//    number of map entries (i.e.: sum of number of items in the stats
	//    bucket's count map).
	//    NOTE: we use the number of items in the count map as the
	//    reference, but in reality, what take place are the
	//    distributions. We are guaranteed the number of entries in the
	//    count map is > than the number of entries in the distributions
	//    maps, so the algorithm is correct, but indeed this means we could
	//    do better.
	nbEntries := 0
	for _, s := range s {
		nbEntries += len(s.Counts)
	}
	if maxEntriesPerPayloads <= 0 || nbEntries < maxEntriesPerPayloads {
		// nothing to do, break early
		return []*stats.Payload{{
			HostName: w.hostname,
			Env:      w.env,
			Stats:    s,
		}}, len(s), nbEntries
	}
	nbPayloads := nbEntries / maxEntriesPerPayloads
	if nbEntries%maxEntriesPerPayloads != 0 {
		nbPayloads++
	}

	type timeWindow struct{ start, duration int64 }
	// 2. Create a slice of nbPayloads maps, mapping a time window (stat +
	//    duration) to a stat bucket. We will build the payloads from these
	//    maps. This allows is to have one stat bucket per time window.
	pMaps := make([]map[timeWindow]stats.Bucket, nbPayloads)
	for i := 0; i < nbPayloads; i++ {
		pMaps[i] = make(map[timeWindow]stats.Bucket, nbPayloads)
	}
	// 3. Iterate over all entries of each stats. Add the entry to one of
	//    the payload container mappings, in a round robin fashion. In some
	//    edge cases, we can end up having the same entry in several
	//    inputted stat buckets. We must check that we never overwrite an
	//    entry in the new stats buckets but cleanly merge instead.
	i := 0
	for _, b := range s {
		tw := timeWindow{b.Start, b.Duration}

		for ekey, e := range b.Counts {
			pm := pMaps[i%nbPayloads]
			newsb, ok := pm[tw]
			if !ok {
				newsb = stats.NewBucket(tw.start, tw.duration)
			}
			pm[tw] = newsb

			if _, ok := newsb.Counts[ekey]; ok {
				newsb.Counts[ekey].Merge(e)
			} else {
				newsb.Counts[ekey] = e
			}

			if _, ok := b.Distributions[ekey]; ok {
				if _, ok := newsb.Distributions[ekey]; ok {
					newsb.Distributions[ekey].Merge(b.Distributions[ekey])
				} else {
					newsb.Distributions[ekey] = b.Distributions[ekey]
				}
			}
			if _, ok := b.ErrDistributions[ekey]; ok {
				if _, ok := newsb.ErrDistributions[ekey]; ok {
					newsb.ErrDistributions[ekey].Merge(b.ErrDistributions[ekey])
				} else {
					newsb.ErrDistributions[ekey] = b.ErrDistributions[ekey]
				}
			}
			i++
		}
	}
	// 4. Create the nbPayloads payloads from the maps.
	nbStats := 0
	nbEntries = 0
	payloads := make([]*stats.Payload, 0, nbPayloads)
	for _, pm := range pMaps {
		pstats := make([]stats.Bucket, 0, len(pm))
		for _, sb := range pm {
			pstats = append(pstats, sb)
			nbEntries += len(sb.Counts)
		}
		payloads = append(payloads, &stats.Payload{
			HostName: w.hostname,
			Env:      w.env,
			Stats:    pstats,
		})

		nbStats += len(pstats)
	}
	return payloads, nbStats, nbEntries
}

var _ eventRecorder = (*StatsWriter)(nil)

func (w *StatsWriter) report() {
	metrics.Count("datadog.trace_agent.stats_writer.payloads", atomic.SwapInt64(&w.stats.Payloads, 0), nil, 1)
	metrics.Count("datadog.trace_agent.stats_writer.stats_buckets", atomic.SwapInt64(&w.stats.StatsBuckets, 0), nil, 1)
	metrics.Count("datadog.trace_agent.stats_writer.bytes", atomic.SwapInt64(&w.stats.Bytes, 0), nil, 1)
	metrics.Count("datadog.trace_agent.stats_writer.retries", atomic.SwapInt64(&w.stats.Retries, 0), nil, 1)
	metrics.Count("datadog.trace_agent.stats_writer.splits", atomic.SwapInt64(&w.stats.Splits, 0), nil, 1)
	metrics.Count("datadog.trace_agent.stats_writer.errors", atomic.SwapInt64(&w.stats.Errors, 0), nil, 1)
}

// recordEvent implements eventRecorder.
func (w *StatsWriter) recordEvent(t eventType, data *eventData) {
	if data != nil {
		metrics.Histogram("datadog.trace_agent.stats_writer.connection_fill", data.connectionFill, nil, 1)
		metrics.Histogram("datadog.trace_agent.stats_writer.queue_fill", data.queueFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Debugf("Retrying to flush stats payload (error: %q)", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeSent:
		log.Debugf("Flushed stats to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.stats_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeRejected:
		log.Warnf("Stats writer payload rejected by edge: %v", data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		w.easylog.Warn("Stats writer queue full. Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		metrics.Count("datadog.trace_agent.stats_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.stats_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}
