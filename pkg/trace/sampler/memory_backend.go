// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryBackend storing any state required to run the sampling algorithms.
//
// Current implementation is only based on counters with polynomial decay.
// Its bias with steady counts is 1 * decayFactor.
// The stored scores represent approximation of the real count values (with a countScaleFactor factor).
type MemoryBackend struct {
	// seen, kept are used for telemetry
	seen int64
	kept int64
	// scores maps signatures to scores.
	scores map[Signature]float64

	// totalScore holds the score sum of all traces (equals the sum of all signature scores).
	totalScore float64

	// sampledScore is the score of all sampled traces.
	sampledScore float64

	// mu is a lock protecting all the scores.
	mu sync.RWMutex

	// DecayPeriod is the time period between each score decay.
	// A lower value is more reactive, but forgets quicker.
	DecayPeriod time.Duration

	// decayFactor is how much we reduce/divide the score at every decay run.
	// A lower value is more reactive, but forgets quicker.
	decayFactor float64

	// countScaleFactor is the factor to apply to move from the score
	// to the representing number of traces per second.
	// By definition of the decay formula is:
	// countScaleFactor = (decayFactor / (decayFactor - 1)) * DecayPeriod
	// It also represents by how much a spike is smoothed: if we instantly
	// receive N times the same signature, its immediate count will be
	// increased by N / countScaleFactor.
	countScaleFactor float64
}

// NewMemoryBackend returns an initialized Backend.
func NewMemoryBackend(decayPeriod time.Duration, decayFactor float64) *MemoryBackend {
	return &MemoryBackend{
		scores:           make(map[Signature]float64),
		DecayPeriod:      decayPeriod,
		decayFactor:      decayFactor,
		countScaleFactor: (decayFactor / (decayFactor - 1)) * decayPeriod.Seconds(),
	}
}

// CountSignature counts an incoming signature.
func (b *MemoryBackend) CountSignature(signature Signature) {
	b.mu.Lock()
	b.scores[signature]++
	b.totalScore++
	atomic.AddInt64(&b.seen, 1)
	b.mu.Unlock()
}

// AddTotalScore adds to the total score.
func (b *MemoryBackend) AddTotalScore(n float64) {
	b.mu.Lock()
	b.totalScore += n
	b.mu.Unlock()
}

// CountSample counts a trace sampled by the sampler.
func (b *MemoryBackend) CountSample() {
	b.mu.Lock()
	b.sampledScore++
	atomic.AddInt64(&b.kept, 1)
	b.mu.Unlock()
}

// CountWeightedSig counts a trace sampled by the sampler.
func (b *MemoryBackend) CountWeightedSig(signature Signature, n float64) {
	b.mu.Lock()
	b.scores[signature] += n
	b.mu.Unlock()
}

func (b *MemoryBackend) report() (int64, int64) {
	kept := atomic.SwapInt64(&b.kept, 0)
	seen := atomic.SwapInt64(&b.seen, 0)
	return kept, seen
}

// GetSignatureScore returns the score of a signature.
// It is normalized to represent a number of signatures per second.
func (b *MemoryBackend) GetSignatureScore(signature Signature) float64 {
	b.mu.RLock()
	score := b.scores[signature] / b.countScaleFactor
	b.mu.RUnlock()

	return score
}

// GetSignatureScores returns the scores for all signatures.
// It is normalized to represent a number of signatures per second.
func (b *MemoryBackend) GetSignatureScores() map[Signature]float64 {
	b.mu.RLock()
	scores := make(map[Signature]float64, len(b.scores))
	for signature, score := range b.scores {
		scores[signature] = score / b.countScaleFactor
	}
	b.mu.RUnlock()

	return scores
}

// GetSampledScore returns the global score of all sampled traces.
func (b *MemoryBackend) GetSampledScore() float64 {
	b.mu.RLock()
	score := b.sampledScore / b.countScaleFactor
	b.mu.RUnlock()

	return score
}

// GetTotalScore returns the global score of all sampled traces.
func (b *MemoryBackend) GetTotalScore() float64 {
	b.mu.RLock()
	score := b.totalScore / b.countScaleFactor
	b.mu.RUnlock()

	return score
}

// GetUpperSampledScore returns a certain upper bound of the global count of all sampled traces.
func (b *MemoryBackend) GetUpperSampledScore() float64 {
	// Overestimate the real score with the high limit of the backend bias.
	return b.GetSampledScore() * b.decayFactor
}

// GetCardinality returns the number of different signatures seen recently.
func (b *MemoryBackend) GetCardinality() int64 {
	b.mu.RLock()
	cardinality := int64(len(b.scores))
	b.mu.RUnlock()

	return cardinality
}

// DecayScore applies the decay to the rolling counters.
func (b *MemoryBackend) DecayScore() {
	b.mu.Lock()
	for sig := range b.scores {
		if b.scores[sig] > b.decayFactor*minSignatureScoreOffset {
			b.scores[sig] /= b.decayFactor
		} else {
			// When the score is too small, we can optimize by simply dropping the entry.
			delete(b.scores, sig)
		}
	}
	b.totalScore /= b.decayFactor
	b.sampledScore /= b.decayFactor
	b.mu.Unlock()
}
