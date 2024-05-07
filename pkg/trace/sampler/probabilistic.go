// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sampler

import (
	"encoding/binary"
	"encoding/hex"
	"hash/fnv"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// These constants exist to match the behavior of the OTEL probabilistic sampler.
	// See: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/6229c6ad1c49e9cc4b41a8aab8cb5a94a7b82ea5/processor/probabilisticsamplerprocessor/tracesprocessor.go#L38-L42
	numProbabilisticBuckets = 0x4000
	bitMaskHashBuckets      = numProbabilisticBuckets - 1
	percentageScaleFactor   = numProbabilisticBuckets / 100.0
)

// ProbabilisticSampler is a sampler that overrides all other samplers,
// it deterministically samples incoming traces by a hash of their trace ID
type ProbabilisticSampler struct {
	enabled                  bool
	hashSeed                 []byte
	scaledSamplingPercentage uint32

	statsd     statsd.ClientInterface
	tracesSeen *atomic.Int64
	tracesKept *atomic.Int64
	tags       []string

	// start/stop synchronization
	stopOnce sync.Once
	stop     chan struct{}
	stopped  chan struct{}
}

// NewProbabilisticSampler returns a new ProbabilisticSampler that deterministically samples
// a given percentage of incoming spans based on their trace ID
func NewProbabilisticSampler(conf *config.AgentConfig, statsd statsd.ClientInterface) *ProbabilisticSampler {
	hashSeedBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(hashSeedBytes, conf.ProbabilisticSamplerHashSeed)
	return &ProbabilisticSampler{
		enabled:                  conf.ProbabilisticSamplerEnabled,
		hashSeed:                 hashSeedBytes,
		scaledSamplingPercentage: uint32(conf.ProbabilisticSamplerSamplingPercentage * percentageScaleFactor),
		statsd:                   statsd,
		tracesSeen:               atomic.NewInt64(0),
		tracesKept:               atomic.NewInt64(0),
		tags:                     []string{"sampler:probabilistic"},
		stop:                     make(chan struct{}),
		stopped:                  make(chan struct{}),
	}
}

// Start starts up the ProbabilisticSamler's support routine, which periodically sends stats.
func (ps *ProbabilisticSampler) Start() {
	if !ps.enabled {
		close(ps.stopped)
		return
	}
	go func() {
		defer watchdog.LogOnPanic(ps.statsd)
		statsTicker := time.NewTicker(10 * time.Second)
		defer statsTicker.Stop()
		for {
			select {
			case <-statsTicker.C:
				ps.report()
			case <-ps.stop:
				ps.report()
				close(ps.stopped)
				return
			}
		}
	}()

}

// Stop shuts down the ProbabilisticSampler's support routine.
func (ps *ProbabilisticSampler) Stop() {
	if !ps.enabled {
		return
	}
	ps.stopOnce.Do(func() {
		close(ps.stop)
		<-ps.stopped
	})
}

// Sample a trace given the chunk's root span, returns true if the trace should be kept
func (ps *ProbabilisticSampler) Sample(root *trace.Span) bool {
	if !ps.enabled {
		return false
	}
	ps.tracesSeen.Add(1)

	tid, err := get128BitTraceID(root)
	if err != nil {
		log.Errorf("Unable to probabilistically sample, failed to determine 128-bit trace ID from incoming span: %v", err)
		return false
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write(ps.hashSeed)
	_, _ = hasher.Write(tid)
	hash := hasher.Sum32()
	keep := hash&bitMaskHashBuckets < ps.scaledSamplingPercentage
	if keep {
		ps.tracesKept.Add(1)
	}
	return keep
}

func (ps *ProbabilisticSampler) report() {
	seen := ps.tracesSeen.Swap(0)
	kept := ps.tracesKept.Swap(0)
	_ = ps.statsd.Count("datadog.trace_agent.sampler.kept", kept, ps.tags, 1)
	_ = ps.statsd.Count("datadog.trace_agent.sampler.seen", seen, ps.tags, 1)
}

func get128BitTraceID(span *trace.Span) ([]byte, error) {
	// If it's an otel span the whole trace ID is in otel.trace
	if tid, ok := span.Meta["otel.trace_id"]; ok {
		bs, err := hex.DecodeString(tid)
		if err != nil {
			return nil, err
		}
		return bs, nil
	}
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], span.TraceID)
	// Get hex encoded upper bits for datadog spans
	// If no value is found we can use the default `0` value as that's what will have been propagated
	if upper, ok := span.Meta["_dd.p.tid"]; ok {
		u, err := strconv.ParseUint(upper, 16, 64)
		if err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint64(tid[:8], u)
	}
	return tid, nil
}
