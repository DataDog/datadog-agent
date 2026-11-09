// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"net/http"

	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// weightedSemaphore is implemented by *semaphore.Weighted.
type weightedSemaphore interface {
	Acquire(ctx context.Context, n int64) error
	Release(n int64)
}

type concurrencySettings interface {
	numWorkers() int
	maxConns() int
}

// concurrencyPolicy controls the number of workers, connections and in-flight
// requests of the domainForwarders.
type concurrencyPolicy interface {
	concurrencySettings
	// newDomain returns the concurrency controller for a single domain.
	newDomain(domain string) concurrencyDomain
}

// concurrencyDomain controls the concurrency of a single domainForwarder.
type concurrencyDomain interface {
	concurrencySettings
	// newRequestSemaphore returns the semaphore limiting in-flight requests
	// of a worker. It may be shared between workers.
	newRequestSemaphore() weightedSemaphore
	// wrapTransport lets the domain observe the responses from the backend.
	wrapTransport(rt http.RoundTripper) http.RoundTripper
	// start is called once, before the workers of the domain start, and stop
	// once, after they have all stopped.
	start()
	stop()
}

func concurrencyPolicyFromConfig(config config.Component, log log.Component) concurrencyPolicy {
	switch policy := config.GetString("forwarder_concurrency_policy"); policy {
	case "dynamic":
		return dynamicConcurrencyPolicy{log: log, settings: dynamicConcurrencySettingsFromConfig(config, log)}
	case "", "static":
	default:
		log.Warnf("Invalid forwarder_concurrency_policy '%s', falling back to 'static'", policy)
	}

	workers := config.GetInt("forwarder_num_workers")

	return staticConcurrencyPolicy{
		workers:     workers,
		connections: workers,
		requests:    maxConcurrentRequestsFromConfig(config, log, "forwarder_max_concurrent_requests"),
	}
}

// newLocalConcurrencyDomainFromConfig returns the concurrency domain to use for
// the local cluster-agent domain, which is served by a single worker and
// connection.
func newLocalConcurrencyDomainFromConfig(config config.Component, log log.Component) concurrencyDomain {
	return staticConcurrencyPolicy{
		workers:     1,
		connections: 1,
		requests:    maxConcurrentRequestsFromConfig(config, log, "forwarder_max_concurrent_requests"),
	}
}

func maxConcurrentRequestsFromConfig(config config.Component, log log.Component, key string) int64 {
	requests := config.GetInt64(key)
	if requests <= 0 {
		log.Warnf("Invalid %s '%d', setting to 1", key, requests)
		requests = 1
	}
	return requests
}

type staticConcurrencyPolicy struct {
	workers     int
	connections int
	requests    int64
}

func (p staticConcurrencyPolicy) numWorkers() int {
	return p.workers
}

func (p staticConcurrencyPolicy) maxConns() int {
	return p.connections
}

func (p staticConcurrencyPolicy) newDomain(_ string) concurrencyDomain {
	return p
}

func (p staticConcurrencyPolicy) newRequestSemaphore() weightedSemaphore {
	return semaphore.NewWeighted(p.requests)
}

func (p staticConcurrencyPolicy) wrapTransport(rt http.RoundTripper) http.RoundTripper {
	return rt
}

func (p staticConcurrencyPolicy) start() {}

func (p staticConcurrencyPolicy) stop() {}
