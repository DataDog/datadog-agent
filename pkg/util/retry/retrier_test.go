// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type DummyLogic struct {
	mock.Mock
	Retrier
}

func (l *DummyLogic) Attempt() error {
	args := l.Called()
	return args.Error(0)
}

func TestSetup(t *testing.T) {
	mocked := &DummyLogic{}
	assert.Equal(t, NeedSetup, mocked.RetryStatus())

	for nb, tc := range []struct {
		config *Config
		err    error
	}{
		{
			// nil config
			config: nil,
			err:    errors.New("nil configuration object"),
		},
		{
			// OneTry strategy
			config: &Config{
				Name:          "mocked",
				AttemptMethod: mocked.Attempt,
				Strategy:      OneTry,
			},
			err: nil,
		},
		{
			// RetryCount no count
			config: &Config{
				Name:          "mocked",
				AttemptMethod: mocked.Attempt,
				Strategy:      RetryCount,
			},
			err: errors.New("RetryCount strategy needs a non-zero RetryCount"),
		},
		{
			// RetryCount no delay
			config: &Config{
				Name:          "mocked",
				AttemptMethod: mocked.Attempt,
				Strategy:      RetryCount,
				RetryCount:    5,
			},
			err: errors.New("RetryCount strategy needs a non-zero RetryDelay"),
		},
		{
			// RetryCount OK
			config: &Config{
				Name:          "mocked",
				AttemptMethod: mocked.Attempt,
				Strategy:      RetryCount,
				RetryCount:    5,
				RetryDelay:    15 * time.Second,
			},
			err: nil,
		},
	} {
		t.Logf("test case %d", nb)
		err := mocked.SetupRetrier(tc.config)
		if tc.err == nil {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
			assert.Contains(t, tc.err.Error(), err.Error())
		}

	}
}

func TestNotInited(t *testing.T) {
	mocked := &DummyLogic{}
	assert.Equal(t, NeedSetup, mocked.RetryStatus())
	err := mocked.TriggerRetry()
	assert.NotNil(t, err)
	_, status := IsRetryError(err)
	assert.Equal(t, NeedSetup, status.RetryStatus)
}

func TestOneTryOK(t *testing.T) {
	mocked := &DummyLogic{}
	mocked.On("Attempt").Return(nil)
	config := &Config{
		Name:          "mocked",
		AttemptMethod: mocked.Attempt,
	}
	err := mocked.SetupRetrier(config)
	assert.Nil(t, err)
	err = mocked.TriggerRetry()
	assert.Nil(t, err)
}

func TestOneTryFail(t *testing.T) {
	mocked := &DummyLogic{}
	mocked.On("Attempt").Return(errors.New("nope"))
	config := &Config{
		Name:          "mocked",
		AttemptMethod: mocked.Attempt,
	}
	err := mocked.SetupRetrier(config)
	assert.Nil(t, err)
	err = mocked.TriggerRetry()
	assert.NotNil(t, err)
	assert.True(t, IsErrPermaFail(err))
}

func TestRetryCount(t *testing.T) {
	mocked := &DummyLogic{}
	mocked.On("Attempt").Return(errors.New("nope"))
	config := &Config{
		Name:          "mocked",
		AttemptMethod: mocked.Attempt,
		Strategy:      RetryCount,
		RetryCount:    5,
		RetryDelay:    100 * time.Nanosecond,
	}
	err := mocked.SetupRetrier(config)
	assert.Nil(t, err)

	// Try 4 times, should return FailWillRetry
	for i := 0; i < 4; i++ {
		err = mocked.TriggerRetry()
		assert.NotNil(t, err)
		assert.True(t, IsErrWillRetry(err))
		time.Sleep(time.Nanosecond) // Make sure we expire the delay
	}

	// 5th time should return PermaFail
	err = mocked.TriggerRetry()
	assert.True(t, IsErrPermaFail(err))
}

func TestRetryDelayNotElapsed(t *testing.T) {
	retryDelay := 20 * time.Minute
	mocked := &DummyLogic{}
	mocked.On("Attempt").Return(errors.New("nope"))
	config := &Config{
		Name:          "mocked",
		AttemptMethod: mocked.Attempt,
		Strategy:      RetryCount,
		RetryCount:    5,
		RetryDelay:    retryDelay,
	}
	err := mocked.SetupRetrier(config)
	assert.Nil(t, err)

	// First call will trigger an attempt
	err = mocked.TriggerRetry()
	assert.True(t, IsErrWillRetry(err))

	// Testing the NextRetry value is within 1ms
	expectedNext := time.Now().Add(retryDelay - 100*time.Millisecond)
	assert.WithinDuration(t, expectedNext, mocked.NextRetry(), time.Millisecond)

	// Second call should skip

	err = mocked.TriggerRetry()
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "try delay not elapsed yet")
}

func TestRetryDelayRecover(t *testing.T) {
	mocked := &DummyLogic{}
	mocked.On("Attempt").Return(errors.New("nope")).Once()
	config := &Config{
		Name:          "mocked",
		AttemptMethod: mocked.Attempt,
		Strategy:      RetryCount,
		RetryCount:    5,
		RetryDelay:    100 * time.Millisecond,
	}
	err := mocked.SetupRetrier(config)
	assert.Nil(t, err)

	// First call will trigger an attempt
	err = mocked.TriggerRetry()
	assert.True(t, IsErrWillRetry(err))
	time.Sleep(time.Nanosecond) // Make sure we expire the delay

	// Second call should return OK
	mocked.On("Attempt").Return(nil)
	err = mocked.TriggerRetry()
	assert.Nil(t, err)
}
