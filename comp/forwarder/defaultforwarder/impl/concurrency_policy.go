// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"

	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// weightedSemaphore is implemented by *semaphore.Weighted.
type weightedSemaphore interface {
	Acquire(ctx context.Context, n int64) error
	Release(n int64)
}

// concurrencyPolicy controls the number of workers, connections and in-flight
// requests of a domainForwarder.
type concurrencyPolicy interface {
	numWorkers() int
	maxConns() int
	// newRequestSemaphore returns the semaphore limiting in-flight requests
	// of a single worker.
	newRequestSemaphore() weightedSemaphore
	// forLocal returns the policy to use for the local cluster-agent domain.
	forLocal() concurrencyPolicy
}

type staticConcurrencyPolicy struct {
	workers     int
	connections int
	requests    int64
}

func concurrencyPolicyFromConfig(config config.Component, log log.Component) staticConcurrencyPolicy {
	workers := config.GetInt("forwarder_num_workers")

	requests := config.GetInt64("forwarder_max_concurrent_requests")
	if requests <= 0 {
		log.Warnf("Invalid forwarder_max_concurrent_requests '%d', setting to 1", requests)
		requests = 1
	}

	return staticConcurrencyPolicy{
		workers:     workers,
		connections: workers,
		requests:    requests,
	}
}

func (p staticConcurrencyPolicy) numWorkers() int {
	return p.workers
}

func (p staticConcurrencyPolicy) maxConns() int {
	return p.connections
}

func (p staticConcurrencyPolicy) newRequestSemaphore() weightedSemaphore {
	return semaphore.NewWeighted(p.requests)
}

func (p staticConcurrencyPolicy) forLocal() concurrencyPolicy {
	p.workers = 1
	p.connections = 1
	return p
}
