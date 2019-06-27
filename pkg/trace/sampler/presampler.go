package sampler

// [TODO:christian] publish all through expvar, but wait until the PR
// with cpu watchdog is merged as there are probably going to be git conflicts...

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PreSamplerStats contains pre-sampler data. The public content
// might be interesting for statistics, logging.
type PreSamplerStats struct {
	// Rate is the target pre-sampling rate.
	Rate float64
	// Error is the last error got when trying to calc the pre-sampling rate.
	// Stored as a string as this is easier to marshal & publish in JSON.
	Error string
	// RecentPayloadsSeen is the number of payloads that passed by.
	RecentPayloadsSeen float64
	// RecentTracesSeen is the number of traces that passed by.
	RecentTracesSeen float64
	// RecentTracesDropped is the number of traces that were dropped.
	RecentTracesDropped float64
}

// PreSampler tries to tell wether we should keep a payload, even
// before fully processing it. Its only clues are the unparsed payload
// and the HTTP headers. It should remain very light and fast.
type PreSampler struct {
	stats       PreSamplerStats
	decayPeriod time.Duration
	decayFactor float64
	mu          sync.RWMutex // needed since many requests can run in parallel
	exit        chan struct{}
}

// NewPreSampler returns an initialized presampler
func NewPreSampler() *PreSampler {
	decayFactor := 9.0 / 8.0
	return &PreSampler{
		stats: PreSamplerStats{
			Rate: 1,
		},
		decayPeriod: defaultDecayPeriod,
		decayFactor: decayFactor,
		exit:        make(chan struct{}),
	}
}

// Run runs and block on the Sampler main loop
func (ps *PreSampler) Run() {
	t := time.NewTicker(ps.decayPeriod)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			ps.decayScore()
		case <-ps.exit:
			return
		}
	}
}

// Stop stops the main Run loop
func (ps *PreSampler) Stop() {
	close(ps.exit)
}

// SetRate set the pre-sample rate, thread-safe.
func (ps *PreSampler) SetRate(rate float64) {
	ps.mu.Lock()
	ps.stats.Rate = rate
	ps.mu.Unlock()
}

// Rate returns the current target pre-sample rate, thread-safe.
// The target pre-sample rate is the value set with SetRate, ideally this
// is the sample rate, but depending on what is received, the real rate
// might defer.
func (ps *PreSampler) Rate() float64 {
	ps.mu.RLock()
	rate := ps.stats.Rate
	ps.mu.RUnlock()
	return rate
}

// SetError set the pre-sample error, thread-safe.
func (ps *PreSampler) SetError(err error) {
	ps.mu.Lock()
	if err != nil {
		ps.stats.Error = err.Error()
	} else {
		ps.stats.Error = ""
	}
	ps.mu.Unlock()
}

// RealRate returns the current real pre-sample rate, thread-safe.
// This is the value obtained by counting what was kept and dropped.
func (ps *PreSampler) RealRate() float64 {
	ps.mu.RLock()
	rate := ps.stats.RealRate()
	ps.mu.RUnlock()
	return rate
}

// RealRate calcuates the current real pre-sample rate from
// the stats data. If no data is available, returns the target rate.
func (stats *PreSamplerStats) RealRate() float64 {
	if stats.RecentTracesSeen <= 0 { // careful with div by 0
		return stats.Rate
	}
	return 1 - (stats.RecentTracesDropped / stats.RecentTracesSeen)
}

// Stats returns a copy of the currrent pre-sampler stats.
func (ps *PreSampler) Stats() *PreSamplerStats {
	ps.mu.RLock()
	stats := ps.stats
	ps.mu.RUnlock()
	return &stats
}

// SampleWithCount tells wether a given payload should be kept (true means: "yes, keep it").
// Calling this alters the statistics, it affects the result of RealRate() so
// only call it once per payload.
func (ps *PreSampler) SampleWithCount(traceCount int64) bool {
	if traceCount <= 0 {
		return true // no sensible value in traceCount, disable pre-sampling
	}

	keep := true

	ps.mu.Lock()

	if ps.stats.RealRate() > ps.stats.Rate {
		// Too many things processed, drop the current payload.
		keep = false
		ps.stats.RecentTracesDropped += float64(traceCount)
	}

	// This should be done *after* testing RealRate() against Rate,
	// else we could end up systematically dropping the first payload.
	ps.stats.RecentPayloadsSeen++
	ps.stats.RecentTracesSeen += float64(traceCount)

	ps.mu.Unlock()

	if !keep {
		log.Debugf("pre-sampling at rate %f dropped payload with %d traces", ps.Rate(), traceCount)
	}

	return keep
}

// decayScore applies the decay to the rolling counters
func (ps *PreSampler) decayScore() {
	ps.mu.Lock()

	ps.stats.RecentPayloadsSeen /= ps.decayFactor
	ps.stats.RecentTracesSeen /= ps.decayFactor
	ps.stats.RecentTracesDropped /= ps.decayFactor

	ps.mu.Unlock()
}

// CalcPreSampleRate gives the new sample rate to apply for a given max user CPU average.
// It takes the current sample rate and user CPU average as those parameters both
// have an influence on the result.
func CalcPreSampleRate(maxUserAvg, currentUserAvg, currentRate float64) (float64, error) {
	const (
		// deltaMin is a threshold that must be passed before changing the
		// pre-sampling rate. If set to 0.1, for example, the new rate must be
		// below 90% or above 110% of the previous value, before we actually
		// adjust the sampling rate. This is to avoid over-adapting and jittering.
		deltaMin = float64(0.15) // +/- 15% change
		// rateMin is an absolute minimum rate, never sample more than this, it is
		// inefficient, the cost handling the payloads without even reading them
		// is too high anyway.
		rateMin = float64(0.05) // 5% hard-limit
	)

	if maxUserAvg <= 0 || currentUserAvg < 0 || currentRate < 0 || currentRate > 1 {
		return 1, fmt.Errorf("inconsistent pre-sampling input maxUserAvg=%f currentUserAvg=%f currentRate=%f",
			maxUserAvg, currentUserAvg, currentRate)
	}
	if currentUserAvg == 0 || currentRate == 0 {
		return 1, nil // not initialized yet, beside, need to return now else divide by zero error
	}

	newRate := currentRate * maxUserAvg / currentUserAvg
	if newRate >= 1 {
		return 1, nil // no need to pre-sample anything
	}

	delta := (newRate - currentRate) / currentRate
	if delta > -deltaMin && delta < deltaMin {
		// no need to change, this is close enough to what we want (avoid jittering)
		return currentRate, nil
	}

	// Taking the average of both values, it is going to converge in the long run,
	// but no need to hurry, wait for next iteration.
	newRate = (newRate + currentRate) / 2

	if newRate < rateMin {
		// Here, we would need a too-aggressive sampling rate to cope with
		// our objective, and pre-sampling is not the right tool any more.
		return rateMin, fmt.Errorf("raising pre-sampling rate from %0.1f %% to %0.1f %%", newRate*100, rateMin*100)
	}

	return newRate, nil
}
