// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

// TimeNow useful for mocking
var TimeNow = time.Now

const (
	Unblocked = iota
	HalfBlocked
	Blocked
)

type circuitBreakerState = int

type block struct {
	nbError int
	until   time.Time
	state   circuitBreakerState
	m       sync.Mutex
}

type blockedEndpoints struct {
	errorPerEndpoint map[string]*block
	backoffPolicy    backoff.Policy
	m                sync.RWMutex
}

func newBlockedEndpoints(config config.Component, log log.Component) *blockedEndpoints {
	backoffFactor := config.GetFloat64("forwarder_backoff_factor")
	if backoffFactor < 2 {
		log.Warnf("Configured forwarder_backoff_factor (%v) is less than 2; 2 will be used", backoffFactor)
		backoffFactor = 2
	}

	backoffBase := config.GetFloat64("forwarder_backoff_base")
	if backoffBase <= 0 {
		log.Warnf("Configured forwarder_backoff_base (%v) is not positive; 2 will be used", backoffBase)
		backoffBase = 2
	}

	backoffMax := config.GetFloat64("forwarder_backoff_max")
	if backoffMax <= 0 {
		log.Warnf("Configured forwarder_backoff_max (%v) is not positive; 64 seconds will be used", backoffMax)
		backoffMax = 64
	}

	recInterval := config.GetInt("forwarder_recovery_interval")
	if recInterval <= 0 {
		log.Warnf("Configured forwarder_recovery_interval (%v) is not positive; %v will be used", recInterval, pkgconfigsetup.DefaultForwarderRecoveryInterval)
		recInterval = pkgconfigsetup.DefaultForwarderRecoveryInterval
	}

	recoveryReset := config.GetBool("forwarder_recovery_reset")

	return &blockedEndpoints{
		errorPerEndpoint: make(map[string]*block),
		backoffPolicy:    backoff.NewExpBackoffPolicy(backoffFactor, backoffBase, backoffMax, recInterval, recoveryReset),
	}
}

func printState(prefix string, state circuitBreakerState) {
	switch state {
	case Unblocked:
		fmt.Println("\033[035m", prefix, "unblocked", "\033[0m")
	case HalfBlocked:
		fmt.Println("\033[035m", prefix, "half blocked", "\033[0m")
	case Blocked:
		fmt.Println("\033[035m", prefix, "blocked", "\033[0m")
	}

}

func (e *block) setState(state circuitBreakerState) {
	e.state = state
	printState("new state", e.state)
}

func (e *blockedEndpoints) getState(endpoint string) (bool, circuitBreakerState) {
	if b, ok := e.errorPerEndpoint[endpoint]; ok {
		return true, b.state
	}

	return false, 0
}

func (e *blockedEndpoints) close(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{
			state: Unblocked,
		}
	}

	switch b.state {
	case Unblocked:
		// The circuit breaker is closed, we need to open it for a given time
		b.nbError = e.backoffPolicy.IncError(b.nbError)
		b.until = TimeNow().Add(e.getBackoffDuration(b.nbError))
		b.setState(Blocked)
	case HalfBlocked:
		// There is a risk that this transaction was sent before the circuit breaker was
		// moved to half open. Currently this assumes that it wasn't and we need to work
		// out a way to solve this....
		// The test transaction failed, so we need to mave back to closed.
		b.nbError = e.backoffPolicy.IncError(b.nbError)
		b.until = TimeNow().Add(e.getBackoffDuration(b.nbError))
		b.setState(Blocked)
	case Blocked:
		// We ignore all failures coming in when open. These are transactions sent
		// before the first one returned with an error.
	}

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) recover(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{
			state: Unblocked,
		}
	}

	switch b.state {
	case Unblocked:
		// Nothing to do, we are already closed and running smoothly.
	case HalfBlocked:
		// The test worked, we can ease off
		b.nbError = e.backoffPolicy.DecError(b.nbError)
		if b.nbError == 0 {
			b.setState(Unblocked)
		} else {
			b.until = TimeNow().Add(e.getBackoffDuration(b.nbError))
			b.setState(Blocked)
		}
	case Blocked:
		// If we are open and a successful transaction came through, we
		// can't be sure if it was sent before the errored transaction or
		// after, so we will ignore this.
	}

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) isBlock(endpoint string) bool {
	e.m.RLock()
	defer e.m.RUnlock()

	if b, ok := e.errorPerEndpoint[endpoint]; ok {

		printState("current state", b.state)
		switch b.state {
		case HalfBlocked:
			// We have already sent the single transactions to test if the endpoint is now up
			return true
		case Blocked:
			if TimeNow().Before(b.until) {
				return true
			} else {
				// Upgrade to a full lock so we can move
				// to HalfOpen and send this transaction
				// to test the endpoint.
				b.m.Lock()
				defer b.m.Unlock()
				if b.state == Blocked {
					b.setState(HalfBlocked)
				}

				return false
			}
		}
	}

	return false
}

func (e *blockedEndpoints) getBackoffDuration(numErrors int) time.Duration {
	return e.backoffPolicy.GetBackoffDuration(numErrors)
}
