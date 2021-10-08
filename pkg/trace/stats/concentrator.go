// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultBufferLen represents the default buffer length; the number of bucket size
// units used by the concentrator.
const defaultBufferLen = 2

// Concentrator produces time bucketed statistics from a stream of raw traces.
// https://en.wikipedia.org/wiki/Knelson_concentrator
// Gets an imperial shitton of traces, and outputs pre-computed data structures
// allowing to find the gold (stats) amongst the traces.
type Concentrator struct {
	In  chan Input
	Out chan pb.StatsPayload

	// bucket duration in nanoseconds
	bsize int64
	// Timestamp of the oldest time bucket for which we allow data.
	// Any ingested stats older than it get added to this bucket.
	oldestTs int64
	// bufferLen is the number of 10s stats bucket we keep in memory before flushing them.
	// It means that we can compute stats only for the last `bufferLen * bsize` and that we
	// wait such time before flushing the stats.
	// This only applies to past buckets. Stats buckets in the future are allowed with no restriction.
	bufferLen     int
	exit          chan struct{}
	exitWG        sync.WaitGroup
	buckets       map[int64]*RawBucket // buckets used to aggregate stats per timestamp
	mu            sync.Mutex
	agentEnv      string
	agentHostname string
}

// NewConcentrator initializes a new concentrator ready to be started
func NewConcentrator(conf *config.AgentConfig, out chan pb.StatsPayload, now time.Time) *Concentrator {
	bsize := conf.BucketInterval.Nanoseconds()
	c := Concentrator{
		bsize:   bsize,
		buckets: make(map[int64]*RawBucket),
		// At start, only allow stats for the current time bucket. Ensure we don't
		// override buckets which could have been sent before an Agent restart.
		oldestTs: alignTs(now.UnixNano(), bsize),
		// TODO: Move to configuration.
		bufferLen:     defaultBufferLen,
		In:            make(chan Input, 100),
		Out:           out,
		exit:          make(chan struct{}),
		agentEnv:      conf.DefaultEnv,
		agentHostname: conf.Hostname,
	}
	return &c
}

// Start starts the concentrator.
func (c *Concentrator) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		c.Run()
	}()
}

// Run runs the main loop of the concentrator goroutine. Traces are received
// through `Add`, this loop only deals with flushing.
func (c *Concentrator) Run() {
	c.exitWG.Add(1)
	defer c.exitWG.Done()

	// flush with the same period as stats buckets
	flushTicker := time.NewTicker(time.Duration(c.bsize) * time.Nanosecond)
	defer flushTicker.Stop()

	log.Debug("Starting concentrator")

	go func() {
		for {
			select {
			case inputs := <-c.In:
				c.Add(inputs)
			}
		}
	}()
	for {
		select {
		case <-flushTicker.C:
			c.Out <- c.Flush()
		case <-c.exit:
			log.Info("Exiting concentrator, computing remaining stats")
			c.Out <- c.Flush()
			return
		}
	}
}

// Stop stops the main Run loop.
func (c *Concentrator) Stop() {
	close(c.exit)
	c.exitWG.Wait()
}

// EnvTrace contains input for the concentractor.
type EnvTrace struct {
	Trace WeightedTrace
	Env   string
}

// Input specifies a set of traces originating from a certain payload.
type Input struct {
	Traces      []EnvTrace
	ContainerID string
}

// Add applies the given input to the concentrator.
func (c *Concentrator) Add(t Input) {
	c.mu.Lock()
	for _, trace := range t.Traces {
		c.addNow(&trace, t.ContainerID)
	}
	c.mu.Unlock()
}

// addNow adds the given input into the concentrator.
// Callers must guard!
func (c *Concentrator) addNow(i *EnvTrace, containerID string) {
	env := i.Env
	if env == "" {
		env = c.agentEnv
	}
	for _, s := range i.Trace {
		if !(s.TopLevel || s.Measured) {
			continue
		}
		end := s.Start + s.Duration
		btime := end - end%c.bsize

		// If too far in the past, count in the oldest-allowed time bucket instead.
		if btime < c.oldestTs {
			btime = c.oldestTs
		}

		b, ok := c.buckets[btime]
		if !ok {
			b = NewRawBucket(uint64(btime), uint64(c.bsize))
			c.buckets[btime] = b
		}
		b.HandleSpan(s, env, c.agentHostname, containerID)
	}
}

// Flush deletes and returns complete statistic buckets
func (c *Concentrator) Flush() pb.StatsPayload {
	return c.flushNow(time.Now().UnixNano())
}

func (c *Concentrator) flushNow(now int64) pb.StatsPayload {
	m := make(map[PayloadAggregationKey][]pb.ClientStatsBucket)

	c.mu.Lock()
	for ts, srb := range c.buckets {
		// Always keep `bufferLen` buckets (default is 2: current + previous one).
		// This is a trade-off: we accept slightly late traces (clock skew and stuff)
		// but we delay flushing by at most `bufferLen` buckets.
		if ts > now-int64(c.bufferLen)*c.bsize {
			continue
		}
		log.Debugf("flushing bucket %d", ts)
		for k, b := range srb.Export() {
			m[k] = append(m[k], b)
		}
		delete(c.buckets, ts)
	}
	// After flushing, update the oldest timestamp allowed to prevent having stats for
	// an already-flushed bucket.
	newOldestTs := alignTs(now, c.bsize) - int64(c.bufferLen-1)*c.bsize
	if newOldestTs > c.oldestTs {
		log.Debugf("update oldestTs to %d", newOldestTs)
		c.oldestTs = newOldestTs
	}
	c.mu.Unlock()
	sb := make([]pb.ClientStatsPayload, 0, len(m))
	for k, s := range m {
		p := pb.ClientStatsPayload{
			Env:         k.Env,
			Hostname:    k.Hostname,
			ContainerID: k.ContainerID,
			Version:     k.Version,
			Stats:       s,
		}
		sb = append(sb, p)
	}
	return pb.StatsPayload{Stats: sb, AgentHostname: c.agentHostname, AgentEnv: c.agentEnv, AgentVersion: info.Version}
}

// alignTs returns the provided timestamp truncated to the bucket size.
// It gives us the start time of the time bucket in which such timestamp falls.
func alignTs(ts int64, bsize int64) int64 {
	return ts - ts%bsize
}
