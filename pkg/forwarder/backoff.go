// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"math"
	"math/rand"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const secondsFloat = float64(time.Second)

var (
	// This controls the overlap between consecutive retry interval ranges. When
	// set to `2`, there is a guarantee that there will be no overlap. The overlap
	// will asymptotically approach 50% the higher the value is set.
	minBackoffFactor float64

	// This controls the rate of exponential growth. Also, you can calculate the start
	// of the very first retry interval range by evaluating the following expression:
	// baseBackoffTime / minBackoffFactor * 2
	baseBackoffTime float64

	// This is the maximum number of seconds to wait for a retry.
	maxBackoffTime float64

	// This controls how many retry interval ranges to step down for an endpoint
	// upon success. Increasing this should only be considered when maxBackoffTime
	// is particularly high or if our intake team is particularly confident.
	recoveryInterval int
)

func init() {
	// We need to load default values
	loadConfig()
}

func loadConfig() {
	backoffFactor := config.Datadog.GetFloat64("forwarder_backoff_factor")
	if backoffFactor >= 2 {
		minBackoffFactor = backoffFactor
	} else {
		minBackoffFactor = 2
		log.Warnf("Configured forwarder_backoff_factor (%v) is less than 2; 2 will be used", backoffFactor)
	}

	backoffBase := config.Datadog.GetFloat64("forwarder_backoff_base")
	if backoffBase > 0 {
		baseBackoffTime = backoffBase
	} else {
		baseBackoffTime = 2
		log.Warnf("Configured forwarder_backoff_base (%v) is not positive; 2 will be used", backoffBase)
	}

	backoffMax := config.Datadog.GetFloat64("forwarder_backoff_max")
	if backoffMax > 0 {
		maxBackoffTime = backoffMax
	} else {
		maxBackoffTime = 64
		log.Warnf("Configured forwarder_backoff_max (%v) is not positive; 64 seconds will be used", backoffMax)
	}

	recInterval := config.Datadog.GetInt("forwarder_recovery_interval")
	if recInterval > 0 {
		recoveryInterval = recInterval
	} else {
		recoveryInterval = 1
		log.Warnf("Configured forwarder_recovery_interval (%v) is not positive; 1 will be used", recInterval)
	}

	recoveryReset := config.Datadog.GetBool("forwarder_recovery_reset")
	if recoveryReset {
		recoveryInterval = maxErrors
	}

	// Calculate how many errors it will take to reach the maxBackoffTime
	maxErrors = int(math.Floor(math.Log2(maxBackoffTime/baseBackoffTime))) + 1
}

func randomBetween(min, max float64) float64 {
	return rand.Float64()*(max-min) + min
}

// GetBackoffDuration returns an appropriate amount of time to wait for the next network
// error retry given the current number of errors. Unlike `github.com/cenkalti/backoff`,
// this implementation is thread-safe.
func GetBackoffDuration(numErrors int) time.Duration {
	var backoffTime float64

	if numErrors > 0 {
		backoffTime = baseBackoffTime * math.Pow(2, float64(numErrors))

		if backoffTime > maxBackoffTime {
			backoffTime = maxBackoffTime
		} else {
			min := backoffTime / minBackoffFactor
			max := math.Min(maxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}
	}

	return time.Duration(backoffTime * secondsFloat)
}
