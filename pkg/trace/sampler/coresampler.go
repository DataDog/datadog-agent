// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"sync/atomic"
	"time"

	ddatomic "github.com/DataDog/datadog-agent/pkg/trace/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const (
	bucketDuration = 10 * time.Second
	numBuckets     = 6
)

// Sampler is the main component of the sampling logic
// Seen traces are counted per signature in a circular buffer
// of numBuckets.
// The sampler distributes uniformly on all signature
// a targetTPS. The bucket with the maximum counts over the period
// of the buffer is used to compute the sampling rates.
type Sampler struct {
	// seen maps signatures to scores.
	seen         map[Signature][numBuckets]uint32
	lastRotation uint64
	// rates maps sampling rate in %
	rates map[Signature]float64

	// muSeen is a lock protecting seen map
	muSeen sync.Mutex
	// muRates is a lock protecting rates map
	muRates sync.RWMutex

	// Maximum limit to the total number of traces per second to sample
	targetTPS *ddatomic.Float64
	// extraRate is an extra raw sampling rate to apply on top of the sampler rate
	extraRate float64

	totalSeen int64
	totalKept int64

	tags    []string
	exit    chan struct{}
	stopped chan struct{}
}

// newSampler returns an initialized Sampler
func newSampler(extraRate float64, targetTPS float64, tags []string) *Sampler {
	s := &Sampler{
		seen: make(map[Signature][numBuckets]uint32),

		extraRate: extraRate,
		targetTPS: ddatomic.NewFloat(targetTPS),
		tags:      tags,

		exit:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	return s
}

// updateTargetTPS updates the max TPS limit
func (s *Sampler) updateTargetTPS(targetTPS float64) {
	s.targetTPS.Store(targetTPS)
}

// Start runs and the Sampler main loop
func (s *Sampler) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		statsTicker := time.NewTicker(10 * time.Second)
		defer statsTicker.Stop()
		for {
			select {
			case <-statsTicker.C:
				s.report()
			case <-s.exit:
				close(s.stopped)
				return
			}
		}
	}()
}

// countWeightedSig counts a trace sampled by the sampler.
func (s *Sampler) countWeightedSig(now time.Time, signature Signature, n uint32) {
	id := (now.Unix() / int64(bucketDuration.Seconds())) % numBuckets
	s.muSeen.Lock()
	s.updateRates(id, id) // TODO
	buckets, ok := s.seen[signature]
	if !ok {
		buckets = [numBuckets]uint32{}
		s.seen[signature] = buckets
	}
	buckets[id] += n
	s.muSeen.Unlock()
	atomic.AddInt64(&s.totalSeen, int64(n))
}

func (s *Sampler) updateRates(previousBucket, newBucket int64) {
	if len(s.seen) == 0 {
		return
	}
	tpsPerSig := s.targetTPS.Load() / float64(len(s.seen))

	rates := make(map[Signature]float64, len(s.seen))
	for sig, buckets := range s.seen {
		maxBucket := uint32(0)

		// TODO zeroes + previous bucket
		for _, c := range buckets {
			if c > maxBucket {
				maxBucket = c
			}
		}
		seenTPS := float64(maxBucket) / bucketDuration.Seconds()
		rate := 1.0
		if tpsPerSig < seenTPS {
			rate = tpsPerSig / seenTPS
		}
		rates[sig] = rate
	}
	s.muRates.Lock()
	s.rates = rates
	s.muRates.Unlock()
}

// countSample counts a trace sampled by the sampler.
func (s *Sampler) countSample() {
	atomic.AddInt64(&s.totalKept, 1)
}

// getSignatureSampleRate returns the sampling rate to apply to a signature
func (s *Sampler) getSignatureSampleRate(sig Signature) float64 {
	s.muRates.RLock()
	defer s.muRates.RUnlock()
	rate, ok := s.rates[sig]
	if !ok {
		return s.extraRate
	}
	return rate * s.extraRate
}

// getAllSignatureSampleRates returns the sampling rate to apply to each signature
func (s *Sampler) getAllSignatureSampleRates() (map[Signature]float64, float64) {
	s.muRates.RLock()
	defer s.muRates.RUnlock()
	rates := make(map[Signature]float64, len(s.rates))
	defaultRate := s.extraRate
	for sig, val := range s.rates {
		val *= s.extraRate
		if val < defaultRate {
			defaultRate = val
		}
		rates[sig] = val
	}
	return rates, defaultRate
}

func (s *Sampler) report() {
	seen := atomic.SwapInt64(&s.totalSeen, 0)
	kept := atomic.SwapInt64(&s.totalKept, 0)
	metrics.Count("datadog.trace_agent.sampler.kept", kept, s.tags, 1)
	metrics.Count("datadog.trace_agent.sampler.seen", seen, s.tags, 1)
}

// Stop stops the main Run loop
func (s *Sampler) Stop() {
	close(s.exit)
	<-s.stopped
}

// getSampleRate returns the sample rate to apply to a trace.
func (s *Sampler) getSampleRate(trace pb.Trace, root *pb.Span, signature Signature) float64 {
	return s.getSignatureSampleRate(signature) * s.extraRate
}
