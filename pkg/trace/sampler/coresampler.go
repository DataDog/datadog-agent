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
	seen map[Signature][numBuckets]uint32
	// lastBucketID is the index of the last bucket on which traces were counted
	lastBucketID int64
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

// updateTargetTPS updates the targetTPS and all rates
func (s *Sampler) updateTargetTPS(targetTPS float64) {
	previousTargetTPS := s.targetTPS.Load()
	s.targetTPS.Store(targetTPS)

	if previousTargetTPS == 0 {
		return
	}
	ratio := targetTPS / previousTargetTPS

	s.muRates.Lock()
	for sig, rate := range s.rates {
		s.rates[sig] = rate * ratio
	}
	s.muRates.Unlock()
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

// countWeightedSig counts a trace sampled by the sampler and update rates
// if buckets are rotated
func (s *Sampler) countWeightedSig(now time.Time, signature Signature, n uint32) {
	bucketID := now.Unix() / int64(bucketDuration.Seconds())
	s.muSeen.Lock()
	prevBucketID := s.lastBucketID
	s.lastBucketID = bucketID

	// pass through each bucket, zero expired ones and adjust sampling rates
	if prevBucketID != bucketID {
		s.updateRates(prevBucketID, bucketID)
	}

	buckets, ok := s.seen[signature]
	if !ok {
		buckets = [numBuckets]uint32{}
	}
	buckets[bucketID%numBuckets] += n
	s.seen[signature] = buckets
	s.muSeen.Unlock()
	atomic.AddInt64(&s.totalSeen, int64(n))
}

// updateRates distributes TPS uniformly on each signature and apply it to the moving
// max of seen buckets.
// Rates increase are bounded by 20% increases, it requires 13 evaluations (1.2**13 = 10.6)
// to increase a sampling rate by 10 fold.
func (s *Sampler) updateRates(previousBucket, newBucket int64) {
	if len(s.seen) == 0 {
		return
	}
	tpsPerSig := s.targetTPS.Load() / float64(len(s.seen))

	s.muRates.Lock()
	defer s.muRates.Unlock()

	rates := make(map[Signature]float64, len(s.seen))
	for sig, buckets := range s.seen {

		maxBucket, buckets := zeroAndGetMax(buckets, previousBucket, newBucket)
		s.seen[sig] = buckets

		seenTPS := float64(maxBucket) / bucketDuration.Seconds()
		rate := 1.0
		if tpsPerSig < seenTPS {
			rate = tpsPerSig / seenTPS
		}
		// caping increase rate to 20%
		if prevRate, ok := s.rates[sig]; ok && prevRate != 0 {
			if rate/prevRate > 1.2 {
				rate = prevRate * 1.2
			}
		}
		if rate > 1.0 {
			rate = 1.0
		}
		// no traffic on this signature, clean it up from the sampler
		if rate == 1.0 && maxBucket == 0 {
			delete(s.seen, sig)
			continue
		}
		rates[sig] = rate
	}
	s.rates = rates
}

// zeroAndGetMax zeroes expired buckets and returns the max count
func zeroAndGetMax(buckets [numBuckets]uint32, previousBucket, newBucket int64) (uint32, [numBuckets]uint32) {
	maxBucket := uint32(0)
	for i := previousBucket + 1; i <= previousBucket+numBuckets; i++ {
		index := i % numBuckets

		// if a complete rotation happened between previousBucket and newBucket
		// all buckets will be zeroed
		if i < newBucket {
			buckets[index] = 0
			continue
		}

		value := buckets[index]
		if value > maxBucket {
			maxBucket = value
		}

		// take in account previous value of the bucket that is overriden
		// in this rotation
		if i == newBucket {
			buckets[index] = 0
		}
	}
	return maxBucket, buckets
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
	return s.getSignatureSampleRate(signature)
}
