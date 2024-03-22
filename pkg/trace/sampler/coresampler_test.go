// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestSamplerAccessRace(_ *testing.T) {
	s := newSampler(1, 2, nil, &statsd.NoOpClient{})
	var wg sync.WaitGroup
	wg.Add(5)
	for j := 0; j < 5; j++ {
		go func(j int) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				s.countWeightedSig(time.Now().Add(time.Duration(5*(j+i))*time.Second), Signature(i%3), 5)
				s.report()
				s.countSample()
				s.getSignatureSampleRate(Signature(i % 3))
				s.getAllSignatureSampleRates()
			}
		}(j)
	}
	wg.Wait()
}

func TestZeroAndGetMaxBuckets(t *testing.T) {
	tts := []struct {
		name            string
		buckets         [numBuckets]float32
		bucketDistance  int64
		previousBucket  int64
		newBucket       int64
		expectedMax     float32
		expectedBuckets [numBuckets]float32
	}{
		{
			name:            "same bucket",
			bucketDistance:  0,
			buckets:         [numBuckets]float32{10, 11, 12, 13, 14, 15},
			expectedMax:     15,
			expectedBuckets: [numBuckets]float32{10, 11, 12, 13, 14, 15},
		},
		{
			name:            "zeroes only",
			bucketDistance:  numBuckets + 1,
			buckets:         [numBuckets]float32{10, 11, 12, 13, 14, 15},
			expectedMax:     0,
			expectedBuckets: [numBuckets]float32{0, 0, 0, 0, 0, 0},
		},
		{
			name:            "max",
			buckets:         [numBuckets]float32{10, 11, 0, 13, 14, 0},
			expectedMax:     14,
			expectedBuckets: [numBuckets]float32{10, 11, 0, 13, 14, 0},
		},
		{
			name:            "3 zeroes",
			buckets:         [numBuckets]float32{10, 11, 17, 13, 14, 19},
			previousBucket:  3,
			newBucket:       7,
			expectedMax:     17,
			expectedBuckets: [numBuckets]float32{0, 0, 17, 13, 0, 0},
		},
		{
			name:            "4 zeroes",
			buckets:         [numBuckets]float32{10, 11, 10, 13, 14, 19},
			previousBucket:  3,
			newBucket:       8,
			expectedMax:     13,
			expectedBuckets: [numBuckets]float32{0, 0, 0, 13, 0, 0},
		},
		{
			name:            "4 zeroes max is new window",
			buckets:         [numBuckets]float32{10, 11, 129191, 13, 14, 19},
			previousBucket:  3,
			newBucket:       8,
			expectedMax:     129191,
			expectedBuckets: [numBuckets]float32{0, 0, 0, 13, 0, 0},
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			if tc.bucketDistance > 0 {
				// test all combinations (each slot + at least one extra rotation)
				for i := 0; i < numBuckets*2; i++ {
					newBucket := tc.bucketDistance + int64(i)
					previousBucket := int64(i)
					max, buckets := zeroAndGetMax(tc.buckets, previousBucket, newBucket)
					assert.Equal(t, tc.expectedMax, max)
					assert.Equal(t, tc.expectedBuckets, buckets)
				}
			} else {
				max, buckets := zeroAndGetMax(tc.buckets, tc.previousBucket, tc.newBucket)
				assert.Equal(t, tc.expectedMax, max)
				assert.Equal(t, tc.expectedBuckets, buckets)

			}
		})
	}
}

func TestRateIncrease(t *testing.T) {
	targetTPS := 7.0
	initialTPS := 21.0
	s := newSampler(1, targetTPS, nil, &statsd.NoOpClient{})

	testSig := Signature(25)
	testTime := time.Now()
	s.countWeightedSig(testTime, testSig, float32(initialTPS*bucketDuration.Seconds()))
	// force rate evaluation
	s.countWeightedSig(testTime.Add(bucketDuration+time.Nanosecond), testSig, 0)

	// move out of the max window
	testTime = testTime.Add(numBuckets*bucketDuration + 1*time.Nanosecond)
	expectedRate := targetTPS / initialTPS
	for i := 0; i <= 10; i++ {
		s.countWeightedSig(testTime.Add(time.Duration(i)*bucketDuration), Signature(0), 1)
		rates, defaultRate := s.getAllSignatureSampleRates()
		assert.Equal(t, expectedRate, defaultRate)

		rate, ok := rates[testSig]
		require.True(t, ok)
		assert.Equal(t, expectedRate, rate)
		assert.Equal(t, expectedRate, s.getSignatureSampleRate(testSig))
		expectedRate *= maxRateIncrease

		if expectedRate > 1 {
			break
		}
	}
}

func TestOldSigEviction(t *testing.T) {
	targetTPS := 7.0
	initialTPS := 21.0
	s := newSampler(1, targetTPS, nil, &statsd.NoOpClient{})

	testSig := Signature(25)
	testTime := time.Now()
	s.countWeightedSig(testTime, testSig, float32(initialTPS*bucketDuration.Seconds()))
	// force rate evaluation
	s.countWeightedSig(testTime.Add(bucketDuration+time.Nanosecond), testSig, 0)

	// move out of the max window
	testTime = testTime.Add(numBuckets*bucketDuration + 1*time.Nanosecond)
	for i := 0; i <= 20; i++ {
		s.countWeightedSig(testTime.Add(time.Duration(i)*bucketDuration), Signature(0), 1)
		if i < 5 {
			rates, _ := s.getAllSignatureSampleRates()
			_, ok := rates[testSig]
			assert.True(t, ok)
			_, ok = s.seen[testSig]
			assert.True(t, ok)
		}
	}
	rates, defaultRate := s.getAllSignatureSampleRates()
	_, ok := rates[testSig]
	assert.False(t, ok)
	assert.Equal(t, defaultRate, 1.0)
	_, ok = s.seen[testSig]
	assert.False(t, ok)
}

func TestMovingMax(t *testing.T) {
	targetTPS := 1.0
	s := newSampler(1, targetTPS, nil, &statsd.NoOpClient{})

	testTime := time.Now()

	steps := []float32{37, 22, 19, 55, 0, 0, 0, 0, 0, 0}
	expectedRates := []float64{0, 1 / 37, 1 / 37, 1 / 37, 1 / 55, 1 / 55, 1 / 55, 1 / 55, 1 / 55, 1 / 55}

	for i := range steps {
		s.countWeightedSig(testTime.Add(time.Duration(i)*bucketDuration), Signature(0), steps[i])
		expectedRate := expectedRates[i]
		if expectedRate == 0 {
			continue
		}
		rates, defaultRate := s.getAllSignatureSampleRates()
		assert.Equal(t, expectedRate, defaultRate)

		if i == len(steps)-1 {
			_, ok := rates[Signature(0)]
			require.False(t, ok)
			assert.Equal(t, expectedRate, s.getSignatureSampleRate(Signature(0)))
		} else {
			rate, ok := rates[Signature(0)]
			require.True(t, ok)
			assert.Equal(t, expectedRate, rate)
			assert.Equal(t, expectedRate, s.getSignatureSampleRate(Signature(0)))
		}
	}
}

func TestComputeTPSPerSig(t *testing.T) {
	tts := []struct {
		name              string
		targetTPS         float64
		seenTPS           []float64
		expectedTPSPerSig float64
	}{
		{
			name:              "zeroes",
			targetTPS:         0,
			seenTPS:           []float64{0, 10, 100, 3, 0},
			expectedTPSPerSig: 0,
		},
		{
			name:              "spread uniformly on active sigs",
			targetTPS:         2,
			seenTPS:           []float64{0, 10, 100, 3, 0},
			expectedTPSPerSig: 2.0 / 3,
		},
		{
			name:              "spread unused",
			targetTPS:         10,
			seenTPS:           []float64{0, 10, 100, 3, 0},
			expectedTPSPerSig: 3.5,
		},
		{
			name:              "spread unused left",
			targetTPS:         23.5,
			seenTPS:           []float64{10, 0, 100, 3, 0},
			expectedTPSPerSig: 10.5,
		},
		{
			name:              "spread unused left",
			targetTPS:         23.5,
			seenTPS:           []float64{10, 0, 100, 3, 0},
			expectedTPSPerSig: 10.5,
		},
		{
			name:              "spread unused left2",
			targetTPS:         53.5,
			seenTPS:           []float64{10, 0, 100, 3, 0},
			expectedTPSPerSig: 40.5,
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectedTPSPerSig == 0 {
				assert.Equal(t, tc.expectedTPSPerSig, computeTPSPerSig(tc.targetTPS, tc.seenTPS))
				return
			}
			assert.InEpsilon(t, tc.expectedTPSPerSig, computeTPSPerSig(tc.targetTPS, tc.seenTPS), 0.00000001)

		})
	}
}

func TestDefaultRate(t *testing.T) {
	targetTPS := 10.0
	s := newSampler(1, targetTPS, nil, &statsd.NoOpClient{})
	s.countWeightedSig(time.Now(), Signature(0), 1000)

	_, defaultRate := s.getAllSignatureSampleRates()
	assert.Equal(t, 1.0/20, defaultRate)
	assert.Equal(t, 1.0/20, s.getSignatureSampleRate(Signature(100)))
}

func TestTargetTPSPerSigUpdate(t *testing.T) {
	targetTPS := 10.0
	s := newSampler(1, targetTPS, nil, &statsd.NoOpClient{})

	testTime := time.Now()

	signaturesInitialTPS := []float32{37, 3, 21, 2921, 5}

	for i, c := range signaturesInitialTPS {
		s.countWeightedSig(testTime, Signature(i), c*float32(bucketDuration.Seconds()))
	}
	// trigger rate computation
	s.countWeightedSig(testTime.Add(bucketDuration+time.Nanosecond), Signature(0), 0)

	tts := []struct {
		name                string
		newTargetTPS        float64
		expectedRates       []float64
		expectedDefaultRate float64
	}{
		{
			name:                "increase rates",
			newTargetTPS:        targetTPS,
			expectedDefaultRate: 2.0 / 2921,
			expectedRates:       []float64{2.0 / 37, 2.0 / 3, 2.0 / 21, 2.0 / 2921, 2.0 / 5},
		},
		{
			name:                "set to 0",
			newTargetTPS:        0,
			expectedDefaultRate: 0,
			expectedRates:       []float64{0, 0, 0, 0, 0},
		},
		{
			name:                "set back",
			newTargetTPS:        targetTPS,
			expectedDefaultRate: 2.0 / 2921,
			expectedRates:       []float64{2.0 / 37, 2.0 / 3, 2.0 / 21, 2.0 / 2921, 2.0 / 5},
		},
	}
	epsilon := 0.0000000001
	for j, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			s.updateTargetTPS(tc.newTargetTPS)
			s.countWeightedSig(testTime.Add(time.Duration(j)*bucketDuration+time.Nanosecond), Signature(0), 0)
			rates, defaultRate := s.getAllSignatureSampleRates()
			if tc.expectedDefaultRate == 0 {
				assert.Equal(t, tc.expectedDefaultRate, defaultRate)
			} else {
				assert.InEpsilon(t, tc.expectedDefaultRate, defaultRate, epsilon)
			}
			for i, expectedRate := range tc.expectedRates {
				rate, ok := rates[Signature(i)]
				assert.True(t, ok)
				if expectedRate == 0 {
					assert.Equal(t, expectedRate, rate)
				} else {
					assert.InEpsilon(t, expectedRate, rate, epsilon)
				}
			}

		})
	}
}
