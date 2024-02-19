// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

type block struct {
	nbError int
	until   time.Time
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

func (e *blockedEndpoints) close(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError = e.backoffPolicy.IncError(b.nbError)
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

	b.nbError = e.backoffPolicy.DecError(b.nbError)
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
	return e.backoffPolicy.GetBackoffDuration(numErrors)
}
