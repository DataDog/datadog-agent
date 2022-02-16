// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"fmt"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestSampler() *Sampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	extraRate := 1.0
	targetTPS := 0.0

	return newSampler(extraRate, targetTPS, nil)
}

func TestSamplerAccessRace(t *testing.T) {
	// regression test: even though the sampler is channel protected, it
	// has getters accessing its fields.
	s := newSampler(1, 2, nil)
	testTime := time.Now()
	go func() {
		for i := 0; i < 10000; i++ {
			s.countWeightedSig(testTime, Signature(i%3), 5)
		}
	}()
	for i := 0; i < 5000; i++ {
		s.countSample()
		s.getSignatureSampleRate(Signature(i % 3))
	}
}

func TestZeroAndGetMaxBuckets(t *testing.T) {
	tts := []struct {
		name            string
		buckets         [numBuckets]uint32
		bucketDistance  int64
		previousBucket  int64
		newBucket       int64
		expectedMax     uint32
		expectedBuckets [numBuckets]uint32
	}{
		{
			name:            "same bucket",
			bucketDistance:  0,
			buckets:         [numBuckets]uint32{10, 11, 12, 13, 14, 15},
			expectedMax:     15,
			expectedBuckets: [numBuckets]uint32{10, 11, 12, 13, 14, 15},
		},
		{
			name:            "zeroes only",
			bucketDistance:  numBuckets + 1,
			buckets:         [numBuckets]uint32{10, 11, 12, 13, 14, 15},
			expectedMax:     0,
			expectedBuckets: [numBuckets]uint32{0, 0, 0, 0, 0, 0},
		},
		{
			name:            "max",
			buckets:         [numBuckets]uint32{10, 11, 0, 13, 14, 0},
			expectedMax:     14,
			expectedBuckets: [numBuckets]uint32{10, 11, 0, 13, 14, 0},
		},
		{
			name:            "3 zeroes",
			buckets:         [numBuckets]uint32{10, 11, 17, 13, 14, 19},
			previousBucket:  3,
			newBucket:       7,
			expectedMax:     17,
			expectedBuckets: [numBuckets]uint32{0, 0, 17, 13, 0, 0},
		},
		{
			name:            "4 zeroes",
			buckets:         [numBuckets]uint32{10, 11, 10, 13, 14, 19},
			previousBucket:  3,
			newBucket:       8,
			expectedMax:     13,
			expectedBuckets: [numBuckets]uint32{0, 0, 0, 13, 0, 0},
		},
		{
			name:            "4 zeroes max is new window",
			buckets:         [numBuckets]uint32{10, 11, 129191, 13, 14, 19},
			previousBucket:  3,
			newBucket:       8,
			expectedMax:     129191,
			expectedBuckets: [numBuckets]uint32{0, 0, 0, 13, 0, 0},
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
	s := newSampler(1, targetTPS, nil)

	testSig := Signature(25)
	testTime := time.Now()
	s.countWeightedSig(testTime, testSig, uint32(initialTPS*bucketDuration.Seconds()))
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

func TestMovingMax(t *testing.T) {
	targetTPS := 1.0
	s := newSampler(1, targetTPS, nil)

	testTime := time.Now()

	steps := []uint32{37, 22, 19, 55, 0, 0, 0, 0, 0, 0}
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
			fmt.Println(i)
			require.True(t, ok)
			assert.Equal(t, expectedRate, rate)
			assert.Equal(t, expectedRate, s.getSignatureSampleRate(Signature(0)))
		}
	}
}
