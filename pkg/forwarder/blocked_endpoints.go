// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package forwarder

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const secondsFloat = float64(time.Second)

func randomBetween(min, max float64) float64 {
	return rand.Float64()*(max-min) + min
}

type block struct {
	nbError int
	until   time.Time
}

type blockedEndpoints struct {
	errorPerEndpoint map[string]*block
	m                sync.RWMutex

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

	// This derived value is the number of errors it will take to reach the maxBackoffTime.
	maxErrors int
}

func newBlockedEndpoints() *blockedEndpoints {
	backoffFactor := config.Datadog.GetFloat64("forwarder_backoff_factor")
	if backoffFactor < 2 {
		log.Warnf("Configured forwarder_backoff_factor (%v) is less than 2; 2 will be used", backoffFactor)
		backoffFactor = 2
	}

	backoffBase := config.Datadog.GetFloat64("forwarder_backoff_base")
	if backoffBase <= 0 {
		log.Warnf("Configured forwarder_backoff_base (%v) is not positive; 2 will be used", backoffBase)
		backoffBase = 2
	}

	backoffMax := config.Datadog.GetFloat64("forwarder_backoff_max")
	if backoffMax <= 0 {
		log.Warnf("Configured forwarder_backoff_max (%v) is not positive; 64 seconds will be used", backoffMax)
		backoffMax = 64
	}

	errorsMax := int(math.Floor(math.Log2(backoffMax/backoffBase))) + 1

	recInterval := config.Datadog.GetInt("forwarder_recovery_interval")
	if recInterval <= 0 {
		log.Warnf("Configured forwarder_recovery_interval (%v) is not positive; %v will be used", recInterval, config.DefaultForwarderRecoveryInterval)
		recInterval = config.DefaultForwarderRecoveryInterval
	}

	recoveryReset := config.Datadog.GetBool("forwarder_recovery_reset")
	if recoveryReset {
		recInterval = errorsMax
	}

	return &blockedEndpoints{
		errorPerEndpoint: make(map[string]*block),
		minBackoffFactor: backoffFactor,
		baseBackoffTime:  backoffBase,
		maxBackoffTime:   backoffMax,
		recoveryInterval: recInterval,
		maxErrors:        errorsMax,
	}
}

func (e *blockedEndpoints) close(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError++
	if b.nbError > e.maxErrors {
		b.nbError = e.maxErrors
	}
	b.until = time.Now().Add(e.getBackoffDuration(b.nbError))

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) recover(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError -= e.recoveryInterval
	if b.nbError < 0 {
		b.nbError = 0
	}
	b.until = time.Now().Add(e.getBackoffDuration(b.nbError))

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) isBlock(endpoint string) bool {
	e.m.RLock()
	defer e.m.RUnlock()

	if b, ok := e.errorPerEndpoint[endpoint]; ok && time.Now().Before(b.until) {
		return true
	}
	return false
}

func (e *blockedEndpoints) getBackoffDuration(numErrors int) time.Duration {
	var backoffTime float64

	if numErrors > 0 {
		backoffTime = e.baseBackoffTime * math.Pow(2, float64(numErrors))

		if backoffTime > e.maxBackoffTime {
			backoffTime = e.maxBackoffTime
		} else {
			min := backoffTime / e.minBackoffFactor
			max := math.Min(e.maxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}
	}

	return time.Duration(backoffTime * secondsFloat)
}
