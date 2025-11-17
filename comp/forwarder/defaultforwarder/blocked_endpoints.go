// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

// A given endpoint can be in one of three states.
//
// `unblocked`:
// All is working well. All transactions are sent to the endpoint.
// If when sending a transaction we receive an error, we increase the error count and
// move into a `blocked` state. A timeout is set according to the backoff policy determined
// by the error count.
//
// `blocked`:
// No transactions are sent until the timeout expires. Any errors/successes received from the
// endpoint are ignored (these are likely transactions sent before we moved to `blocked`).
//
// Once the timeout expires, we send a single transaction to the endpoint, and move into a
// `halfBlocked` state.
//
// `halfBlocked`:
// We have sent a single test transaction. We don't want to send anymore transactions until we
// receive a result from the endpoint to indicate if it is healthy.
//
// If the endpoint still returns an error, we increase the error count and move back to `blocked`.
// The increased error count will likely mean the timeout is for an even longer time.
//
// If the endpoint returns a success, we decrease the error count. If the error count is > 0,
// we move back to `blocked`. The reduced error count will mean the timeout is for a shorter time,
// but it prevents the agent from flooding an endpoint as it starts up following an error.
//
// When the error count is back to 0 we return to `unblocked`.
const (
	unblocked circuitBreakerState = iota
	halfBlocked
	blocked
)

type circuitBreakerState int

type block struct {
	endpoint string
	nbError  int
	until    time.Time
	state    circuitBreakerState
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

func (b *block) setState(state circuitBreakerState) {
	b.state = state
}

func (e *blockedEndpoints) getState(endpoint string) (bool, circuitBreakerState) {
	if b, ok := e.errorPerEndpoint[endpoint]; ok {
		return true, b.state
	}

	return false, 0
}

// close is called when we have received an error from this endpoint.
func (e *blockedEndpoints) close(endpoint string, timeNow time.Time) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{
			endpoint: endpoint,
			state:    unblocked,
		}
	}

	switch b.state {
	case unblocked, halfBlocked:
		// If unblocked, this is the first error encountered, if half blocked it means
		// the test transaction failed. In both cases, we need to block for a time.
		b.nbError = e.backoffPolicy.IncError(b.nbError)
		b.until = timeNow.Add(e.getBackoffDuration(b.nbError))
		b.setState(blocked)
	case blocked:
		// We ignore all failures coming in when blocked. These are transactions sent
		// before the first one returned with an error.
	}

	e.errorPerEndpoint[endpoint] = b
}

// recover is called when we have received an success from this endpoint.
func (e *blockedEndpoints) recover(endpoint string, timeNow time.Time) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{
			endpoint: endpoint,
			state:    unblocked,
		}
	}

	switch b.state {
	case unblocked:
		// Nothing to do, we are already unblocked and running smoothly.
	case halfBlocked:
		// The test worked, we can ease off.
		b.nbError = e.backoffPolicy.DecError(b.nbError)
		if b.nbError == 0 {
			b.setState(unblocked)
		} else {
			b.until = timeNow.Add(e.getBackoffDuration(b.nbError))
			b.setState(blocked)
		}
	case blocked:
		// If we are blocked and a successful transaction came through, we
		// can't be sure if it was sent before the errored transaction or
		// after, so we will ignore this.
	}

	e.errorPerEndpoint[endpoint] = b
}

type retryBlockList struct {
	e           *blockedEndpoints
	blockedList map[string]bool
}

// startRetry will start a new check for blocked endpoints when retrying transactions.
// This must be called at the start of the retry loop since we need to treat all the
// transactions to a given endpoint as a whole, either all are sent, or none if the
// endpoint is blocked, or a single one if we just want to send a test transaction.
//
// NOTE: This is not thread safe, `retryBlockList` must not be shared between threads.
func (e *blockedEndpoints) startRetry() *retryBlockList {
	return &retryBlockList{
		e:           e,
		blockedList: make(map[string]bool),
	}
}

// isBlockForRetry checks if the endpoint is blocked when deciding if we want
// to add the transaction to the retry queue.
//
// We check if an endpoint is blocked in two places:
//
// 1. When deciding if we want to requeue a transaction for the worked.
// 2. When the worker is deciding whether to send the transaction.
//
// When adding to the retry queue, we have a list of transactions that are to
// be retried. They are sorted in priority order. If we are `blocked` and the timeout
// has expired, we want to push a single (preferably high priority) transaction to the
// worker to send a test transaction.
// If we are `blocked` and the timeout has not expired, or we are `halfblocked`
// (waiting for the results of the the test transaction) we block all transactions.
func (r *retryBlockList) isBlockForRetry(endpoint string, timeNow time.Time) bool {
	if isblocked, ok := r.blockedList[endpoint]; ok {
		return isblocked
	}

	r.e.m.RLock()
	defer r.e.m.RUnlock()

	if b, ok := r.e.errorPerEndpoint[endpoint]; ok {
		if b.state == blocked {
			if timeNow.Before(b.until) {
				r.blockedList[endpoint] = true
				return true
			}

			// Time has expired, allow one test transaction through, block all
			// ensuing transactions.
			r.blockedList[endpoint] = true
			return false
		} else if b.state == halfBlocked {
			// Wait for the test transaction results
			r.blockedList[endpoint] = true
			return true
		}
	}

	return false
}

// isBlockForSend checks if the endpoint is blocked when deciding if
// we want to send a transaction.
// This function can modify the state. When in `Blocked` after the
// timeout has expired we want to move to `HalfOpen` where we send a
// single transaction to test if the endpoint now available.
func (e *blockedEndpoints) isBlockForSend(endpoint string, timeNow time.Time) bool {
	e.m.Lock()
	defer e.m.Unlock()

	if b, ok := e.errorPerEndpoint[endpoint]; ok {
		switch b.state {
		case halfBlocked:
			// We have already sent the single transactions to test if the endpoint is now up.
			return true
		case blocked:
			if timeNow.Before(b.until) {
				return true
			} else {
				// The timeout has expired, move to `halfBlocked` and send this transaction.
				b.setState(halfBlocked)
				return false
			}
		}
	}
	return false
}

func (e *blockedEndpoints) getBackoffDuration(numErrors int) time.Duration {
	return e.backoffPolicy.GetBackoffDuration(numErrors)
}
