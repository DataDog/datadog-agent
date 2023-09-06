// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flush

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAtTheEnd(t *testing.T) {
	assert := assert.New(t)

	s := &AtTheEnd{}
	assert.False(s.ShouldFlush(Starting, time.Now()), "it should not flush because it's the start of the invocation")
	assert.True(s.ShouldFlush(Stopping, time.Now()), "it should flush because it's the end of the function invocation")
	assert.True(s.ShouldFlush(Stopping, time.Now()), "it shouldn't have memory and should flush again")
	assert.False(s.ShouldFlush(Starting, time.Now().Add(time.Second)), "it should not flush because it's the start of the invocation")
	assert.True(s.ShouldFlush(Stopping, time.Now().Add(time.Second)), "it shouldn't have memory and should flush again")
}

func TestPeriodically(t *testing.T) {
	assert := assert.New(t)

	// flush should happen at least every 2 second
	s := &Periodically{interval: 2 * time.Second}
	s.lastFlush = time.Now().Add(-time.Second * 10)

	assert.True(s.ShouldFlush(Starting, time.Now()), "it should flush because last flush was 10 seconds ago")

	s.lastFlush = time.Now().Add(-time.Second * 60)
	assert.True(s.ShouldFlush(Starting, time.Now()), "it should flush because last flush was 1 minute ago")

	s.lastFlush = time.Now().Add(-time.Second)
	assert.False(s.ShouldFlush(Starting, time.Now()), "it should not flush because last flush was less than 2 second ago")
}

func TestStrategyFromString(t *testing.T) {
	assert := assert.New(t)

	s, err := StrategyFromString("end")
	assert.Equal("end", s.String())
	assert.NoError(err, "parsing this string shouldn't fail")

	s, err = StrategyFromString("periodically")
	assert.Equal("periodically,10000", s.String())
	assert.Equal(s.(*Periodically).interval, 10*time.Second, "default value should be 10s")
	assert.NoError(err, "parsing this string shouldn't fail")

	s, err = StrategyFromString("periodically,10000")
	assert.Equal("periodically,10000", s.String())
	assert.Equal(s.(*Periodically).interval, 10*time.Second, "default value should be 10s")
	assert.NoError(err, "parsing this string shouldn't fail")

	s, err = StrategyFromString("periodically,2000")
	assert.Equal("periodically,2000", s.String())
	assert.Equal(s.(*Periodically).interval, 2*time.Second, "should be 2s")
	assert.NoError(err, "parsing this string shouldn't fail")

	s, err = StrategyFromString("periodically,4789")
	assert.Equal("periodically,4789", s.String())
	assert.Equal(s.(*Periodically).interval, 4789*time.Millisecond, "should be 4.789s")
	assert.NoError(err, "parsing this string shouldn't fail")

	s, err = StrategyFromString("ddog")
	assert.Equal("end", s.String())
	assert.Error(err, "parsing this string should fail")
}

func TestSkipAfterFailure(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	afterLessThanRetryTimeout := now.Add(30 * time.Second)
	afterMoreThanRetryTimeout := now.Add(2 * time.Minute)

	sEnd := &AtTheEnd{}
	assert.True(sEnd.ShouldFlush(Stopping, now), "it should flush because it didn't fail")
	for i := 1; i <= 5; i++ {
		sEnd.Failure(now)
	}
	assert.False(sEnd.ShouldFlush(Stopping, afterLessThanRetryTimeout), "it should not flush because it failed early before")
	assert.True(sEnd.ShouldFlush(Stopping, afterMoreThanRetryTimeout), "it flush because enough time has passed since failure")

	sPeriodic := &Periodically{}
	sPeriodic.Success() // reset global var
	assert.True(sPeriodic.ShouldFlush(Starting, now), "it should flush because it's not the end of the function invocation")
	for i := 1; i <= 5; i++ {
		sPeriodic.Failure(now)
	}
	assert.False(sPeriodic.ShouldFlush(Starting, afterLessThanRetryTimeout), "it should not flush because it failed right away")
	assert.True(sPeriodic.ShouldFlush(Starting, afterMoreThanRetryTimeout), "it flush because enough time has passed since failure")

	t.Cleanup(func() {
		sPeriodic.Success() // reset global var
	})
}

func TestMaxBackoff(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	afterLessThanRetryTimeout := now.Add(4 * time.Minute)
	afterMoreThanRetryTimeout := now.Add(maxBackoffRetrySeconds * time.Second)

	sEnd := &AtTheEnd{}
	assert.True(sEnd.ShouldFlush(Stopping, now), "it should flush because it's not the end of the function invocation")
	for i := 1; i <= 500; i++ {
		sEnd.Failure(now)
	}
	assert.False(sEnd.ShouldFlush(Stopping, afterLessThanRetryTimeout), "it should not flush because it failed right away")
	assert.True(sEnd.ShouldFlush(Stopping, afterMoreThanRetryTimeout), "it flush because more than max backoff passed")

	t.Cleanup(func() {
		sEnd.Success() // reset global var
	})
}
