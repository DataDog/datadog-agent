// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package retry

import (
	"errors"
	"time"
)

// Accumulate values over a period of time.
// Use bucket of `bucketDuration` for the implementation.
type timeIntervalAccumulator struct {
	bucketSum    []int64
	currentIndex int

	currentIndexTime int64
	startIndexTime   int64

	sum            int64
	buckSizeInSecs int64
}

var invalidTime = int64(-1)

// `historyDuration` and `bucketDuration` are rounded to second.
func newTimeIntervalAccumulator(historyDuration time.Duration, bucketDuration time.Duration) (*timeIntervalAccumulator, error) {
	if bucketDuration.Seconds() <= 0 {
		return nil, errors.New("`bucketDuration` must be at least one second")
	}
	if historyDuration.Seconds() < bucketDuration.Seconds() {
		return nil, errors.New("`historyDuration` must be greater or equals to `bucketDuration`")
	}

	bucketCount := int64(historyDuration.Seconds() / bucketDuration.Seconds())
	return &timeIntervalAccumulator{
		bucketSum:        make([]int64, bucketCount),
		currentIndexTime: invalidTime,
		startIndexTime:   invalidTime,
		buckSizeInSecs:   int64(bucketDuration.Seconds()),
	}, nil
}

// This function assumes `now` increases or remains constant between each call as
// the value is always added to the last bucket.
func (a *timeIntervalAccumulator) add(now time.Time, value int64) {
	t := now.Unix()
	if a.startIndexTime == invalidTime {
		a.startIndexTime = t
		a.currentIndexTime = t
	}

	for t >= a.currentIndexTime+a.buckSizeInSecs {
		a.currentIndex = (a.currentIndex + 1) % len(a.bucketSum)
		a.sum -= a.bucketSum[a.currentIndex]
		a.bucketSum[a.currentIndex] = 0

		a.currentIndexTime += a.buckSizeInSecs
		if a.currentIndexTime >= a.startIndexTime+int64(len(a.bucketSum))*a.buckSizeInSecs {
			a.startIndexTime += a.buckSizeInSecs
		}
	}

	a.bucketSum[a.currentIndex] += value
	a.sum += value
}

func (a *timeIntervalAccumulator) getDuration(now time.Time) (int64, time.Duration) {
	// Add `+ 1` as a bucket has a one second resolution.
	// a.startIndexTime represents the time interval [a.startIndexTime, a.startIndexTime+1[
	duration := now.Unix() - a.startIndexTime + 1
	return a.sum, time.Duration(duration) * time.Second
}
