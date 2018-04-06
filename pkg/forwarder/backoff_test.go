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

	"github.com/DataDog/datadog-agent/pkg/config"
)

func init() {
	rand.Seed(10)
}

func TestMinBackoffFactorValid(t *testing.T) {
	// Verify default triggered by init()
	defaultValue := minBackoffFactor
	assert.Equal(t, float64(2), defaultValue)

	// Reset original value when finished
	defer loadConfig()
	defer config.Datadog.Set("forwarder_backoff_factor", defaultValue)

	// Verify configuration updates global var
	config.Datadog.Set("forwarder_backoff_factor", 4)
	loadConfig()
	assert.Equal(t, float64(4), minBackoffFactor)

	// Verify invalid values recover gracefully
	config.Datadog.Set("forwarder_backoff_factor", 1.5)
	loadConfig()
	assert.Equal(t, defaultValue, minBackoffFactor)
}

func TestBaseBackoffTimeValid(t *testing.T) {
	// Verify default triggered by init()
	defaultValue := baseBackoffTime
	assert.Equal(t, float64(2), defaultValue)

	// Reset original value when finished
	defer loadConfig()
	defer config.Datadog.Set("forwarder_backoff_base", defaultValue)

	// Verify configuration updates global var
	config.Datadog.Set("forwarder_backoff_base", 4)
	loadConfig()
	assert.Equal(t, float64(4), baseBackoffTime)

	// Verify invalid values recover gracefully
	config.Datadog.Set("forwarder_backoff_base", 0)
	loadConfig()
	assert.Equal(t, defaultValue, baseBackoffTime)
}

func TestMaxBackoffTimeValid(t *testing.T) {
	// Verify default triggered by init()
	defaultValue := maxBackoffTime
	assert.Equal(t, float64(64), defaultValue)

	// Reset original value when finished
	defer loadConfig()
	defer config.Datadog.Set("forwarder_backoff_max", defaultValue)

	// Verify configuration updates global var
	config.Datadog.Set("forwarder_backoff_max", 128)
	loadConfig()
	assert.Equal(t, float64(128), maxBackoffTime)

	// Verify invalid values recover gracefully
	config.Datadog.Set("forwarder_backoff_max", 0)
	loadConfig()
	assert.Equal(t, defaultValue, maxBackoffTime)
}

func TestRecoveryIntervalValid(t *testing.T) {
	// Verify default triggered by init()
	defaultValue := recoveryInterval
	recoveryReset := config.Datadog.GetBool("forwarder_recovery_reset")
	assert.Equal(t, 1, defaultValue)
	assert.Equal(t, false, recoveryReset)

	// Reset original value when finished
	defer loadConfig()
	defer config.Datadog.Set("forwarder_recovery_reset", recoveryReset)
	defer config.Datadog.Set("forwarder_recovery_interval", defaultValue)

	// Verify configuration updates global var
	config.Datadog.Set("forwarder_recovery_interval", 2)
	loadConfig()
	assert.Equal(t, 2, recoveryInterval)

	// Verify invalid values recover gracefully
	config.Datadog.Set("forwarder_recovery_interval", 0)
	loadConfig()
	assert.Equal(t, defaultValue, recoveryInterval)

	// Verify reset error count
	config.Datadog.Set("forwarder_recovery_reset", true)
	loadConfig()
	assert.Equal(t, maxErrors, recoveryInterval)
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

// Test we increase delay on average
func TestGetBackoffDurationIncrease(t *testing.T) {
	previousBackoffDuration := time.Duration(0) * time.Second
	backoffIncrease := 0
	backoffDecrease := 0

	for i := 1; ; i++ {
		backoffDuration := GetBackoffDuration(i)

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
	backoffDuration := GetBackoffDuration(100)

	assert.Equal(t, time.Duration(maxBackoffTime)*time.Second, backoffDuration)
}
