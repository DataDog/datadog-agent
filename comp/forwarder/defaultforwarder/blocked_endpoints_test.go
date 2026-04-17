// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// 1 Error will give us a backoff duration of base backoff time:
// between
// forwarder_backoff_base * 2 ^ num_errors / forwarder_backoff_factor
// and
// forwarder_backoff_base * 2 ^ num_errors

package defaultforwarder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

func TestMinBackoffFactorValid(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	// Verify default
	defaultValue := policy.MinBackoffFactor
	assert.Equal(t, float64(2), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetInTest("forwarder_backoff_factor", 4)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(4), policy.MinBackoffFactor)

	// Verify invalid values recover gracefully
	mockConfig.SetInTest("forwarder_backoff_factor", 1.5)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.MinBackoffFactor)
}

func TestBaseBackoffTimeValid(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.BaseBackoffTime
	assert.Equal(t, float64(2), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetInTest("forwarder_backoff_base", 4)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(4), policy.BaseBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.SetInTest("forwarder_backoff_base", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.BaseBackoffTime)
}

func TestMaxBackoffTimeValid(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.MaxBackoffTime
	assert.Equal(t, float64(64), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetInTest("forwarder_backoff_max", 128)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(128), policy.MaxBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.SetInTest("forwarder_backoff_max", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.MaxBackoffTime)
}

func TestRecoveryIntervalValid(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.RecoveryInterval
	recoveryReset := mockConfig.GetBool("forwarder_recovery_reset")
	assert.Equal(t, 2, defaultValue)
	assert.Equal(t, false, recoveryReset)

	// Verify configuration updates global var
	mockConfig.SetInTest("forwarder_recovery_interval", 1)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, 1, policy.RecoveryInterval)

	// Verify invalid values recover gracefully
	mockConfig.SetInTest("forwarder_recovery_interval", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.RecoveryInterval)

	// Verify reset error count
	mockConfig.SetInTest("forwarder_recovery_reset", true)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, policy.MaxErrors, policy.RecoveryInterval)
}

// Test we increase delay on average
func TestGetBackoffDurationIncrease(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)
	previousBackoffDuration := time.Duration(0) * time.Second
	backoffIncrease := 0
	backoffDecrease := 0

	for i := 1; ; i++ {
		backoffDuration := e.getBackoffDuration(i)

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if backoffDuration == previousBackoffDuration {
			break
		} else {
			if backoffDuration > previousBackoffDuration {
				backoffIncrease++
			} else {
				backoffDecrease++
			}
			previousBackoffDuration = backoffDuration
		}
	}

	assert.True(t, backoffIncrease >= backoffDecrease)
}

func TestMaxGetBackoffDuration(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)
	backoffDuration := e.getBackoffDuration(100)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	assert.Equal(t, time.Duration(policy.MaxBackoffTime)*time.Second, backoffDuration)
}

func TestMaxErrors(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)
	previousBackoffDuration := time.Duration(0) * time.Second
	attempts := 0

	for i := 1; ; i++ {
		backoffDuration := e.getBackoffDuration(i)

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if backoffDuration == previousBackoffDuration {
			attempts = i - 1
			break
		}

		previousBackoffDuration = backoffDuration
	}

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	assert.Equal(t, policy.MaxErrors, attempts)
}

func TestBlock(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	now := time.Now()
	e.close("test", now)

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, now.Before(e.errorPerEndpoint["test"].until))
}

func TestMaxBlock(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	e.close("test", time.Now())
	e.errorPerEndpoint["test"].nbError = 1000000
	e.errorPerEndpoint["test"].state = halfBlocked

	e.close("test", time.Now())
	now := time.Now()

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	maxBackoffDuration := time.Duration(policy.MaxBackoffTime) * time.Second

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.Equal(t, policy.MaxErrors, e.errorPerEndpoint["test"].nbError)
	assert.True(t, now.Add(maxBackoffDuration).After(e.errorPerEndpoint["test"].until) ||
		now.Add(maxBackoffDuration).Equal(e.errorPerEndpoint["test"].until))
}

func assertState(t *testing.T, e *blockedEndpoints, endpoint string, expected circuitBreakerState) {
	exists, state := e.getState(endpoint)
	assert.True(t, exists)
	assert.Equal(t, expected, state)
}

func TestIsBlockForSendEndpointStaysClosedAfterFailedTest(t *testing.T) {
	mocktime := time.Now()

	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_backoff_base", 1)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlockForSend("test", mocktime))

	e.close("test", mocktime)

	assert.True(t, e.isBlockForSend("test", mocktime))

	mocktime = mocktime.Add(2 * time.Second)
	assert.False(t, e.isBlockForSend("test", mocktime))
	assert.True(t, e.isBlockForSend("test", mocktime))

	e.close("test", mocktime)

	assertState(t, e, "test", blocked)

	// Still blocked after 2 seconds
	mocktime = mocktime.Add(2 * time.Second)

	assertState(t, e, "test", blocked)

	// Testing again after another 2 seconds
	mocktime = mocktime.Add(2 * time.Second)
	assert.False(t, e.isBlockForSend("test", mocktime))
	assert.True(t, e.isBlockForSend("test", mocktime))
	assertState(t, e, "test", halfBlocked)
}

func TestIsBlockForSendEndpointReopensAfterSuccessfulTest(t *testing.T) {
	mocktime := time.Now()

	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_backoff_base", 1)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlockForSend("test", mocktime))

	e.close("test", mocktime)

	assert.True(t, e.isBlockForSend("test", mocktime))

	mocktime = mocktime.Add(2 * time.Second)
	assert.False(t, e.isBlockForSend("test", mocktime))
	assert.True(t, e.isBlockForSend("test", mocktime))

	e.recover("test", mocktime)

	e.isBlockForSend("test", mocktime)
	assertState(t, e, "test", unblocked)
}

func TestIsBlockForSendEndpointReopensForTest(t *testing.T) {
	mocktime := time.Now()

	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_backoff_base", 1)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlockForSend("test", mocktime))

	e.close("test", mocktime)

	assert.True(t, e.isBlockForSend("test", mocktime))

	mocktime = mocktime.Add(2 * time.Second)
	assert.False(t, e.isBlockForSend("test", mocktime))
	assert.True(t, e.isBlockForSend("test", mocktime))
}

func TestIsBlockForSendEndpointCloses(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlockForSend("test", time.Now()))

	e.close("test", time.Now())

	assert.True(t, e.isBlockForSend("test", time.Now()))
}

func TestIsBlockForSendOpen(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlockForSend("test", time.Now()))
}

func TestIsBlockForRetryOpen(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	retry := e.startRetry()

	// Without any errored transactions occurring, we are unblocked no matter
	// how many times we call.
	assert.False(t, retry.isBlockForRetry("test", time.Now()))
	assert.False(t, retry.isBlockForRetry("test", time.Now()))
	assert.False(t, retry.isBlockForRetry("test", time.Now()))
}

func TestIsBlockForRetryCloses(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	mocktime := time.Now()
	e.close("test", mocktime)
	retry := e.startRetry()

	// If the first call to `isBlockForRetry` is within the blocked time the enpoint
	// is blocked as will all subsequent calls.
	assert.True(t, retry.isBlockForRetry("test", mocktime))

	expired := e.errorPerEndpoint["test"].until

	assert.True(t, retry.isBlockForRetry("test", expired.Add(time.Second)))
	assert.True(t, retry.isBlockForRetry("test", expired.Add(2*time.Second)))
}

func TestIsBlockForRetrySendsSingleTransactionInHalfBlockedPeriod(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	mocktime := time.Now()
	e.close("test", mocktime)
	retry := e.startRetry()

	// If the first call to `isBlockForRetry` is after the expired time, we unblock a
	// single transaction
	expired := e.errorPerEndpoint["test"].until
	assert.False(t, retry.isBlockForRetry("test", expired.Add(time.Second)))
	assert.True(t, retry.isBlockForRetry("test", expired.Add(2*time.Second)))
	assert.True(t, retry.isBlockForRetry("test", expired.Add(2*time.Second)))
}

func TestIsBlockForRetryReopens(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	e := newBlockedEndpoints(mockConfig, log)

	e.close("test", time.Now())
	retry := e.startRetry()

	// If the first call to `isBlockForRetry` is after the expired time, we unblock a
	// single transaction
	expired := e.errorPerEndpoint["test"].until
	assert.False(t, retry.isBlockForRetry("test", expired.Add(time.Second)))

	// That test transaction is successful, so the endpoint should be recovered.
	mocktime := expired.Add(time.Second)
	// `isBlockForSend` call is needed to move the state into `HalfBlocked`.
	e.isBlockForSend("test", mocktime)
	e.recover("test", mocktime)

	// That endpoint should be open again
	retry = e.startRetry()
	assert.False(t, retry.isBlockForRetry("test", mocktime.Add(time.Second)))
	assert.False(t, retry.isBlockForRetry("test", mocktime.Add(time.Second)))
	assert.False(t, retry.isBlockForRetry("test", mocktime.Add(time.Second)))
}
