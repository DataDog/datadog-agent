package sampler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcPreSampleRate(t *testing.T) {
	assert := assert.New(t)

	// [0] -> maxUserAvg: the value in the conf file
	// [1] -> currentUserAvg: the value reported by the CPU watchdog
	// [2] -> currentRate: the current (pre)sampling rate
	expected := map[struct {
		maxUserAvg     float64 // the value in the conf file
		currentUserAvg float64 // the value reported by the CPU watchdog
		currentRate    float64 // the current (pre)sampling rate
	}]struct {
		r   float64
		err error
	}{
		// Various cases showing general usage
		{maxUserAvg: 0.1, currentUserAvg: 0.1, currentRate: 1}:     {r: 1, err: nil},
		{maxUserAvg: 0.2, currentUserAvg: 0.1, currentRate: 1}:     {r: 1, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.15, currentRate: 1}:    {r: 0.8333333333333334, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.2, currentRate: 1}:     {r: 0.75, err: nil},
		{maxUserAvg: 0.2, currentUserAvg: 1, currentRate: 1}:       {r: 0.6, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.11, currentRate: 1}:    {r: 1, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.09, currentRate: 1}:    {r: 1, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.05, currentRate: 1}:    {r: 1, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.11, currentRate: 0.5}:  {r: 0.5, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.5, currentRate: 0.5}:   {r: 0.3, err: nil},
		{maxUserAvg: 0.15, currentUserAvg: 0.05, currentRate: 0.5}: {r: 1, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 0.05, currentRate: 0.1}:  {r: 0.15000000000000002, err: nil},
		{maxUserAvg: 0.04, currentUserAvg: 0.05, currentRate: 1}:   {r: 0.8999999999999999, err: nil},
		{maxUserAvg: 0.025, currentUserAvg: 0.05, currentRate: 1}:  {r: 0.75, err: nil},
		{maxUserAvg: 0.01, currentUserAvg: 0.05, currentRate: 0.1}: {r: 0.060000000000000005, err: nil},

		// Check it's back to 1 even if current sampling rate is close to 1
		{maxUserAvg: 0.01, currentUserAvg: 0.005, currentRate: 0.99}: {r: 1, err: nil},

		// Anti-jittering thing (not doing anything if target is too close to current)
		{maxUserAvg: 5, currentUserAvg: 3, currentRate: 0.5}:   {r: 0.6666666666666667, err: nil},
		{maxUserAvg: 5, currentUserAvg: 4, currentRate: 0.5}:   {r: 0.5625, err: nil},
		{maxUserAvg: 5, currentUserAvg: 4.5, currentRate: 0.5}: {r: 0.5, err: nil},
		{maxUserAvg: 5, currentUserAvg: 4.9, currentRate: 0.5}: {r: 0.5, err: nil},
		{maxUserAvg: 5, currentUserAvg: 5, currentRate: 0.5}:   {r: 0.5, err: nil},
		{maxUserAvg: 5, currentUserAvg: 5.1, currentRate: 0.5}: {r: 0.5, err: nil},
		{maxUserAvg: 5, currentUserAvg: 5.5, currentRate: 0.5}: {r: 0.5, err: nil},
		{maxUserAvg: 5, currentUserAvg: 6, currentRate: 0.5}:   {r: 0.45833333333333337, err: nil},
		{maxUserAvg: 5, currentUserAvg: 7, currentRate: 0.5}:   {r: 0.4285714285714286, err: nil},

		// What happens when sampling at very high rate, and how do we converge on this
		{maxUserAvg: 0.1, currentUserAvg: 1000000, currentRate: 1}:                  {r: 0.50000005, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 500000, currentRate: 0.50000005}:          {r: 0.25000007500000504, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 250000, currentRate: 0.25000007500000504}: {r: 0.1250000875000175, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 125000, currentRate: 0.1250000875000175}:  {r: 0.06250009375004376, err: nil},
		{maxUserAvg: 0.1, currentUserAvg: 65000, currentRate: 0.06250009375004376}:  {r: 0.05, err: fmt.Errorf("raising pre-sampling rate from 3.1 %% to 5.0 %%")},
		{maxUserAvg: 0.1, currentUserAvg: 50000, currentRate: 0.05}:                 {r: 0.05, err: fmt.Errorf("raising pre-sampling rate from 2.5 %% to 5.0 %%")},

		// not initialized yet, this is what happens at startup (no error, just default to 1)
		{maxUserAvg: 0.1, currentUserAvg: 0, currentRate: 0}: {r: 1, err: nil},

		// invalid input, those should really *NEVER* happen, test is just defensive
		{maxUserAvg: 0, currentUserAvg: 0.1, currentRate: 0.1}:     {r: 1, err: fmt.Errorf("inconsistent pre-sampling input maxUserAvg=0.000000 currentUserAvg=0.100000 currentRate=0.100000")},
		{maxUserAvg: 0.1, currentUserAvg: -0.02, currentRate: 0.1}: {r: 1, err: fmt.Errorf("inconsistent pre-sampling input maxUserAvg=0.100000 currentUserAvg=-0.020000 currentRate=0.100000")},
		{maxUserAvg: 0.1, currentUserAvg: 0.05, currentRate: -0.2}: {r: 1, err: fmt.Errorf("inconsistent pre-sampling input maxUserAvg=0.100000 currentUserAvg=0.050000 currentRate=-0.200000")},
	}

	for k, v := range expected {
		r, err := CalcPreSampleRate(k.maxUserAvg, k.currentUserAvg, k.currentRate)
		assert.Equal(v.r, r, "bad pre sample rate for maxUserAvg=%f currentUserAvg=%f, currentRate=%f, got %v, expected %v", k.maxUserAvg, k.currentUserAvg, k.currentRate, r, v.r)
		if v.err == nil {
			assert.Nil(err, "there should be no error for maxUserAvg=%f currentUserAvg=%f, currentRate=%f, got %v", k.maxUserAvg, k.currentUserAvg, k.currentRate, err)
		} else {
			assert.Equal(v.err, err, "unexpected error for maxUserAvg=%f currentUserAvg=%f, currentRate=%f, got %v, expected %v", k.maxUserAvg, k.currentUserAvg, k.currentRate, err, v.err)
		}
	}
}

func TestPreSamplerRace(t *testing.T) {
	var wg sync.WaitGroup

	const N = 1000
	ps := NewPreSampler()
	wg.Add(5)

	go func() {
		for i := 0; i < N; i++ {
			ps.SetRate(0.5)
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.Rate()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.RealRate()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			_ = ps.SampleWithCount(42)
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < N; i++ {
			ps.decayScore()
			time.Sleep(time.Microsecond)
		}
		wg.Done()
	}()
	wg.Wait()
}

func TestPreSamplerSampleWithCount(t *testing.T) {
	assert := assert.New(t)

	ps := NewPreSampler()
	ps.SetRate(0.2)
	assert.Equal(0.2, ps.RealRate(), "by default, RealRate returns wished rate")
	assert.True(ps.SampleWithCount(100), "always accept first payload")
	ps.decayScore()
	assert.False(ps.SampleWithCount(10), "refuse as this accepting this would make 100%")
	ps.decayScore()
	assert.Equal(0.898876404494382, ps.RealRate())
	assert.False(ps.SampleWithCount(290), "still refuse")
	ps.decayScore()
	assert.False(ps.SampleWithCount(99), "just below the limit")
	ps.decayScore()
	assert.True(ps.SampleWithCount(1), "should there be no decay, this one would be dropped, but with decay, the rate decreased as the recently dropped gain importance over the old initially accepted")
	ps.decayScore()
	assert.Equal(0.16365162139216005, ps.RealRate(), "well below 20%, again, decay speaks")
	assert.True(ps.SampleWithCount(1000000), "accepting payload with many traces")
	ps.decayScore()
	assert.Equal(0.9997119577953764, ps.RealRate(), "real rate is almost 1, as we accepted a hudge payload")
	assert.False(ps.SampleWithCount(100000), "rejecting, real rate is too high now")
	ps.decayScore()
	assert.Equal(0.8986487877795845, ps.RealRate(), "real rate should be now around 90%")
	assert.Equal(PreSamplerStats{
		Rate:                0.2,
		RecentPayloadsSeen:  4.492300911839488, // seen more than this... but decay in action
		RecentTracesSeen:    879284.5615616576,
		RecentTracesDropped: 89116.55620097058,
	}, ps.stats)
}

func TestPreSamplerError(t *testing.T) {
	assert := assert.New(t)

	ps := NewPreSampler()
	assert.Equal("", ps.stats.Error, "fresh pre-sampler should have no error")
	ps.SetError(fmt.Errorf("bad news"))
	assert.Equal("bad news", ps.stats.Error, `error should be "bad news"`)
	ps.SetError(nil)
	assert.Equal("", ps.stats.Error, "after reset, error should be empty")
}
