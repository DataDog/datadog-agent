// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/benbjohnson/clock"
	"go.uber.org/atomic"
)

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Name     string    `json:"name"`
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Tags     string    `json:"tags"`
}

type DsdServerDebug struct {
	sync.Mutex
	Enabled *atomic.Bool
	Stats   map[ckey.ContextKey]metricStat `json:"stats"`
	// counting number of metrics processed last X seconds
	metricsCounts metricsCountBuckets
	// keyGen is used to generate hashes of the metrics received by dogstatsd
	keyGen *ckey.KeyGenerator

	// clock is used to keep a consistent time state within the debug server whether
	// we use a real clock in production code or a mock clock for unit testing
	clock           clock.Clock
	tagsAccumulator *tagset.HashingTagsAccumulator
}

// newDSDServerDebug creates a new instance of a DsdServerDebug
func newDSDServerDebug() *DsdServerDebug {
	return newDSDServerDebugWithClock(clock.New())
}

// newDSDServerDebugWithClock creates a new instance of a DsdServerDebug with a specific clock
// It is used to create a DsdServerDebug with a real clock for production code and with a mock clock for testing code
func newDSDServerDebugWithClock(clock clock.Clock) *DsdServerDebug {
	return &DsdServerDebug{
		Enabled: atomic.NewBool(false),
		Stats:   make(map[ckey.ContextKey]metricStat),
		metricsCounts: metricsCountBuckets{
			counts:     [5]uint64{0, 0, 0, 0, 0},
			metricChan: make(chan struct{}),
			closeChan:  make(chan struct{}),
		},
		keyGen: ckey.NewKeyGenerator(),
		clock:  clock,
	}
}

// metricsCountBuckets is counting the amount of metrics received for the last 5 seconds.
// It is used to detect spikes.
type metricsCountBuckets struct {
	counts     [5]uint64
	bucketIdx  int
	currentSec time.Time
	metricChan chan struct{}
	closeChan  chan struct{}
}

// FormatDebugStats returns a printable version of debug stats.
func FormatDebugStats(stats []byte) (string, error) {
	var dogStats map[uint64]metricStat
	if err := json.Unmarshal(stats, &dogStats); err != nil {
		return "", err
	}

	// put metrics in order: first is the more frequent
	order := make([]uint64, len(dogStats))
	i := 0
	for metric := range dogStats {
		order[i] = metric
		i++
	}

	sort.Slice(order, func(i, j int) bool {
		return dogStats[order[i]].Count > dogStats[order[j]].Count
	})

	// write the response
	buf := bytes.NewBuffer(nil)

	header := fmt.Sprintf("%-40s | %-20s | %-10s | %-20s\n", "Metric", "Tags", "Count", "Last Seen")
	buf.Write([]byte(header))
	buf.Write([]byte(strings.Repeat("-", len(header)) + "\n"))

	for _, key := range order {
		stats := dogStats[key]
		buf.Write([]byte(fmt.Sprintf("%-40s | %-20s | %-10d | %-20v\n", stats.Name, stats.Tags, stats.Count, stats.LastSeen)))
	}

	if len(dogStats) == 0 {
		buf.Write([]byte("No metrics processed yet."))
	}

	return buf.String(), nil
}

// storeMetricStats stores stats on the given metric sample.
//
// It can help troubleshooting clients with bad behaviors.
func (d *DsdServerDebug) storeMetricStats(sample metrics.MetricSample) {
	now := d.clock.Now()
	d.Lock()
	defer d.Unlock()

	if d.tagsAccumulator == nil {
		d.tagsAccumulator = tagset.NewHashingTagsAccumulator()
	}

	// key
	defer d.tagsAccumulator.Reset()
	d.tagsAccumulator.Append(sample.Tags...)
	key := d.keyGen.Generate(sample.Name, "", d.tagsAccumulator)

	// store
	ms := d.Stats[key]
	ms.Count++
	ms.LastSeen = now
	ms.Name = sample.Name
	ms.Tags = strings.Join(d.tagsAccumulator.Get(), " ") // we don't want/need to share the underlying array
	d.Stats[key] = ms

	d.metricsCounts.metricChan <- struct{}{}
}
