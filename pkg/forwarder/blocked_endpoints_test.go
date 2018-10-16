// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func init() {
	rand.Seed(10)
}

func TestRandomBetween(t *testing.T) {
	getRandomMinMax := func() (float64, float64) {
		a := float64(rand.Intn(10))
		b := float64(rand.Intn(10))
		min := math.Min(a, b)
		max := math.Max(a, b)
		return min, max
	}

	for i := 1; i < 100; i++ {
		min, max := getRandomMinMax()
		between := randomBetween(min, max)

		assert.True(t, min <= between)
		assert.True(t, max >= between)
	}
}

func TestMinBackoffFactorValid(t *testing.T) {
	mockConfig := config.NewMock()
	e := newBlockedEndpoints()

	// Verify default
	defaultValue := e.minBackoffFactor
	assert.Equal(t, float64(2), defaultValue)

	// Reset original value when finished
	defer mockConfig.Set("forwarder_backoff_factor", defaultValue)

	// Verify configuration updates global var
	mockConfig.Set("forwarder_backoff_factor", 4)
	e = newBlockedEndpoints()
	assert.Equal(t, float64(4), e.minBackoffFactor)

	// Verify invalid values recover gracefully
	mockConfig.Set("forwarder_backoff_factor", 1.5)
	e = newBlockedEndpoints()
	assert.Equal(t, defaultValue, e.minBackoffFactor)
}

func TestBaseBackoffTimeValid(t *testing.T) {
	mockConfig := config.NewMock()
	e := newBlockedEndpoints()

	// Verify default
	defaultValue := e.baseBackoffTime
	assert.Equal(t, float64(2), defaultValue)

	// Reset original value when finished
	defer mockConfig.Set("forwarder_backoff_base", defaultValue)

	// Verify configuration updates global var
	mockConfig.Set("forwarder_backoff_base", 4)
	e = newBlockedEndpoints()
	assert.Equal(t, float64(4), e.baseBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.Set("forwarder_backoff_base", 0)
	e = newBlockedEndpoints()
	assert.Equal(t, defaultValue, e.baseBackoffTime)
}

func TestMaxBackoffTimeValid(t *testing.T) {
	mockConfig := config.NewMock()
	e := newBlockedEndpoints()

	// Verify default
	defaultValue := e.maxBackoffTime
	assert.Equal(t, float64(64), defaultValue)

	// Reset original value when finished
	defer mockConfig.Set("forwarder_backoff_max", defaultValue)

	// Verify configuration updates global var
	mockConfig.Set("forwarder_backoff_max", 128)
	e = newBlockedEndpoints()
	assert.Equal(t, float64(128), e.maxBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.Set("forwarder_backoff_max", 0)
	e = newBlockedEndpoints()
	assert.Equal(t, defaultValue, e.maxBackoffTime)
}

func TestRecoveryIntervalValid(t *testing.T) {
	mockConfig := config.NewMock()
	e := newBlockedEndpoints()

	// Verify default
	defaultValue := e.recoveryInterval
	recoveryReset := config.Datadog.GetBool("forwarder_recovery_reset")
	assert.Equal(t, 2, defaultValue)
	assert.Equal(t, false, recoveryReset)

	// Reset original values when finished
	defer mockConfig.Set("forwarder_recovery_reset", recoveryReset)
	defer mockConfig.Set("forwarder_recovery_interval", defaultValue)

	// Verify configuration updates global var
	mockConfig.Set("forwarder_recovery_interval", 1)
	e = newBlockedEndpoints()
	assert.Equal(t, 1, e.recoveryInterval)

	// Verify invalid values recover gracefully
	mockConfig.Set("forwarder_recovery_interval", 0)
	e = newBlockedEndpoints()
	assert.Equal(t, defaultValue, e.recoveryInterval)

	// Verify reset error count
	mockConfig.Set("forwarder_recovery_reset", true)
	e = newBlockedEndpoints()
	assert.Equal(t, e.maxErrors, e.recoveryInterval)
}

// Test we increase delay on average
func TestGetBackoffDurationIncrease(t *testing.T) {
	e := newBlockedEndpoints()
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
	e := newBlockedEndpoints()
	backoffDuration := e.getBackoffDuration(100)

	assert.Equal(t, time.Duration(e.maxBackoffTime)*time.Second, backoffDuration)
}

func TestMaxErrors(t *testing.T) {
	e := newBlockedEndpoints()
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

	assert.Equal(t, e.maxErrors, attempts)
}

func TestBlock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, now.Before(e.errorPerEndpoint["test"].until))
}

func TestMaxBlock(t *testing.T) {
	e := newBlockedEndpoints()
	e.close("test")
	e.errorPerEndpoint["test"].nbError = 1000000

	e.close("test")
	now := time.Now()

	maxBackoffDuration := time.Duration(e.maxBackoffTime) * time.Second

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.Equal(t, e.maxErrors, e.errorPerEndpoint["test"].nbError)
	assert.True(t, now.Add(maxBackoffDuration).After(e.errorPerEndpoint["test"].until) ||
		now.Add(maxBackoffDuration).Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	require.Contains(t, e.errorPerEndpoint, "test")
	e.close("test")
	e.close("test")
	e.close("test")
	e.close("test")

	e.recover("test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == int(math.Max(0, float64(5-e.recoveryInterval))))
}

func TestMaxUnblock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	e.recover("test")
	e.recover("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
	assert.True(t, now.After(e.errorPerEndpoint["test"].until) || now.Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	e.recover("test")
	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
}

func TestIsBlock(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))

	e.close("test")
	assert.True(t, e.isBlock("test"))

	e.recover("test")
	assert.False(t, e.isBlock("test"))
}

func TestIsBlockTiming(t *testing.T) {
	e := newBlockedEndpoints()

	// setting an old close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(-time.Duration(30 * time.Second))}
	assert.False(t, e.isBlock("test"))

	// setting an new close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(time.Duration(30 * time.Second))}
	assert.True(t, e.isBlock("test"))
}

func TestIsblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))
}
