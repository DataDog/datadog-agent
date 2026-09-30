// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingClient is a statsd client recording every Count, Gauge and Distribution call
type recordingClient struct {
	statsd.NoOpClient

	mu            sync.Mutex
	counts        map[string]int64 // summed over calls
	countCalls    int
	gauges        map[string]float64 // last value
	distributions map[string][]float64
}

var _ statsd.ClientInterface = (*recordingClient)(nil)

func newRecordingClient() *recordingClient {
	return &recordingClient{
		counts:        make(map[string]int64),
		gauges:        make(map[string]float64),
		distributions: make(map[string][]float64),
	}
}

func metricKey(name string, tags []string) string {
	if len(tags) == 0 {
		return name
	}
	return name + "|" + strings.Join(tags, ",")
}

func (c *recordingClient) Count(name string, value int64, tags []string, _ float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[metricKey(name, tags)] += value
	c.countCalls++
	return nil
}

func (c *recordingClient) Gauge(name string, value float64, tags []string, _ float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gauges[metricKey(name, tags)] = value
	return nil
}

func (c *recordingClient) Distribution(name string, value float64, tags []string, _ float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := metricKey(name, tags)
	c.distributions[key] = append(c.distributions[key], value)
	return nil
}

// takeCounts returns the counts recorded since the previous call
func (c *recordingClient) takeCounts() (map[string]int64, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts, calls := c.counts, c.countCalls
	c.counts, c.countCalls = make(map[string]int64), 0
	return counts, calls
}

func (c *recordingClient) gauge(key string) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.gauges[key]
	return v, ok
}

type fixedQueuePool struct {
	depth int
}

func (p *fixedQueuePool) Submit(ScanJob) bool { return false }
func (p *fixedQueuePool) QueueDepth() int     { return p.depth }

func TestMetricsCounterDeltas(t *testing.T) {
	client := newRecordingClient()
	stats := &Stats{}
	m := NewMetrics(client, stats, nil, nil, MetricsOptions{Tags: []string{"env:test"}})

	stats.ExecsReceived.Add(10)
	stats.IdentityHits.Add(7)
	stats.ShaHits.Add(1)
	stats.Reads.Add(2)
	stats.TooBig.Add(1)
	stats.NotRegular.Add(1)
	stats.Scans.Add(1)
	stats.Matches.Add(1)
	stats.ScanErrors.Add(1)
	stats.ScanTimeouts.Add(1)
	stats.QueueDrops.Add(1)
	m.flush()

	counts, _ := client.takeCounts()
	assert.Equal(t, map[string]int64{
		metricExecsReceived + "|env:test": 10,
		metricIdentityHits + "|env:test":  7,
		metricShaHits + "|env:test":       1,
		metricReads + "|env:test":         2,
		metricTooBig + "|env:test":        1,
		metricNotRegular + "|env:test":    1,
		metricScans + "|env:test":         1,
		metricMatches + "|env:test":       1,
		metricScanErrors + "|env:test":    1,
		metricScanTimeouts + "|env:test":  1,
		metricQueueDrops + "|env:test":    1,
	}, counts)

	// the second flush sends deltas only, and nothing for unchanged counters
	stats.ExecsReceived.Add(5)
	stats.Scans.Add(2)
	m.flush()

	counts, calls := client.takeCounts()
	assert.Equal(t, map[string]int64{
		metricExecsReceived + "|env:test": 5,
		metricScans + "|env:test":         2,
	}, counts)
	assert.Equal(t, 2, calls)

	// nothing changed: nothing is sent
	m.flush()
	_, calls = client.takeCounts()
	assert.Zero(t, calls)
}

func TestMetricsReadErrorReasons(t *testing.T) {
	client := newRecordingClient()
	stats := &Stats{}
	m := NewMetrics(client, stats, nil, nil, MetricsOptions{})

	stats.IncReadError(ReadErrorNotFound)
	stats.IncReadError(ReadErrorNotFound)
	stats.IncReadError(ReadErrorPermission)
	stats.IncReadError(ReadErrorOther)
	stats.IncReadError(ReadErrorReason(42)) // out of range, counted as other
	m.flush()

	counts, _ := client.takeCounts()
	assert.Equal(t, map[string]int64{
		metricReadErrors + "|reason:not_found":  2,
		metricReadErrors + "|reason:permission": 1,
		metricReadErrors + "|reason:other":      2,
	}, counts)

	stats.IncReadError(ReadErrorPermission)
	m.flush()
	counts, _ = client.takeCounts()
	assert.Equal(t, map[string]int64{
		metricReadErrors + "|reason:permission": 1,
	}, counts)
}

func TestMetricsGauges(t *testing.T) {
	client := newRecordingClient()
	stats := &Stats{}
	deduper := NewStandInDeduper()
	pool := &fixedQueuePool{depth: 3}
	m := NewMetrics(client, stats, deduper, pool, MetricsOptions{})

	now := time.Now()
	deduper.MarkIdentity(Identity{Inode: 1}, now)
	deduper.MarkIdentity(Identity{Inode: 2}, now)
	deduper.ClaimHash([32]byte{1})
	m.flush()

	v, ok := client.gauge(metricIdentityCacheSize)
	require.True(t, ok)
	assert.Equal(t, 2.0, v)
	v, ok = client.gauge(metricShaSetSize)
	require.True(t, ok)
	assert.Equal(t, 1.0, v)
	v, ok = client.gauge(metricQueueDepth)
	require.True(t, ok)
	assert.Equal(t, 3.0, v)

	// gauges are re-sent at every flush with the current values
	deduper.ReleaseHash([32]byte{1})
	pool.depth = 0
	m.flush()

	v, _ = client.gauge(metricShaSetSize)
	assert.Equal(t, 0.0, v)
	v, _ = client.gauge(metricQueueDepth)
	assert.Equal(t, 0.0, v)
}

func TestMetricsNilGaugeSources(t *testing.T) {
	client := newRecordingClient()
	m := NewMetrics(client, &Stats{}, nil, nil, MetricsOptions{})
	m.flush()

	_, ok := client.gauge(metricIdentityCacheSize)
	assert.False(t, ok)
	_, ok = client.gauge(metricQueueDepth)
	assert.False(t, ok)
}

func TestScanDurationObserver(t *testing.T) {
	client := newRecordingClient()
	tags := []string{"env:test"}
	stats := &Stats{ObserveScanDuration: NewScanDurationObserver(client, tags)}
	tags[0] = "mutated" // the observer keeps its own copy

	stats.ScanDone(1500 * time.Millisecond)
	stats.ScanDone(250 * time.Millisecond)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, []float64{1.5, 0.25}, client.distributions[metricScanDuration+"|env:test"])
}

func TestMetricsStartStop(t *testing.T) {
	client := newRecordingClient()
	stats := &Stats{}
	m := NewMetrics(client, stats, nil, &fixedQueuePool{}, MetricsOptions{Interval: 5 * time.Millisecond})

	m.Start(context.Background())

	// counters are emitted by the periodic flush, concurrently with the stages counting
	stats.ExecsReceived.Add(3)
	require.Eventually(t, func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return client.counts[metricExecsReceived] == 3
	}, 5*time.Second, time.Millisecond)

	// counts added right before Stop are sent by its final flush
	stats.ExecsReceived.Add(4)
	m.Stop()

	client.mu.Lock()
	assert.Equal(t, int64(7), client.counts[metricExecsReceived])
	client.mu.Unlock()

	// the goroutine is gone: nothing is sent after Stop
	client.takeCounts()
	stats.ExecsReceived.Add(1)
	time.Sleep(20 * time.Millisecond)
	_, calls := client.takeCounts()
	assert.Zero(t, calls)
}

func TestMetricsStopsOnContextDone(t *testing.T) {
	m := NewMetrics(newRecordingClient(), &Stats{}, nil, nil, MetricsOptions{Interval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("flush goroutine did not exit on context cancellation")
	}
	m.Stop()
}

func TestMetricsStopWithoutStart(_ *testing.T) {
	m := NewMetrics(newRecordingClient(), &Stats{}, nil, nil, MetricsOptions{})
	m.Stop()
}
