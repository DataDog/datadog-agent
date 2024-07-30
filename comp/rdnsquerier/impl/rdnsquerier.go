// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the rdnsquerier component interface
package rdnsquerierimpl

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierimplnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
)

// Requires defines the dependencies for the rdnsquerier component
type Requires struct {
	Lifecycle   compdef.Lifecycle
	AgentConfig config.Component
	Logger      log.Component
	Telemetry   telemetry.Component
}

// Provides defines the output of the rdnsquerier component
type Provides struct {
	Comp rdnsquerier.Component
}

type rdnsQuery struct {
	addr           string
	updateHostname func(string)
}

const moduleName = "reverse_dns_enrichment"

type rdnsQuerierTelemetry = struct {
	total              telemetry.Counter
	private            telemetry.Counter
	chanAdded          telemetry.Counter
	droppedChanFull    telemetry.Counter
	droppedRateLimiter telemetry.Counter
	invalidIPAddress   telemetry.Counter
	lookupErrNotFound  telemetry.Counter
	lookupErrTimeout   telemetry.Counter
	lookupErrTemporary telemetry.Counter
	lookupErrOther     telemetry.Counter
	successful         telemetry.Counter
}

type rdnsQuerierImpl struct {
	logger            log.Component
	config            *rdnsQuerierConfig
	internalTelemetry *rdnsQuerierTelemetry

	started     bool
	resolver    resolver
	rateLimiter rateLimiter

	rdnsQueryChan chan *rdnsQuery
	wg            sync.WaitGroup
	cancel        context.CancelFunc
}

// NewComponent creates a new rdnsquerier component
func NewComponent(reqs Requires) (Provides, error) {
	config := newConfig(reqs.AgentConfig)
	reqs.Logger.Infof("Reverse DNS Enrichment config: (enabled=%t workers=%d chan_size=%d rate_limiter.enabled=%t rate_limiter.limit_per_sec=%d cache.enabled=%t cache.entry_ttl=%d cache.clean_interval=%d cache.persist_interval=%d)",
		config.enabled,
		config.workers,
		config.chanSize,
		config.rateLimiterEnabled,
		config.rateLimitPerSec,
		config.cacheEnabled,
		config.cacheEntryTTL,
		config.cacheCleanInterval,
		config.cachePersistInterval)

	if !config.enabled {
		return Provides{
			Comp: rdnsquerierimplnone.NewNone().Comp,
		}, nil
	}

	internalTelemetry := &rdnsQuerierTelemetry{
		reqs.Telemetry.NewCounter(moduleName, "total", []string{}, "Counter measuring the total number of rDNS requests"),
		reqs.Telemetry.NewCounter(moduleName, "private", []string{}, "Counter measuring the number of rDNS requests in the private address space"),
		reqs.Telemetry.NewCounter(moduleName, "chan_added", []string{}, "Counter measuring the number of rDNS requests added to the channel"),
		reqs.Telemetry.NewCounter(moduleName, "dropped_chan_full", []string{}, "Counter measuring the number of rDNS requests dropped because the channel was full"),
		reqs.Telemetry.NewCounter(moduleName, "dropped_rate_limiter", []string{}, "Counter measuring the number of rDNS requests dropped because the rate limiter wait failed"),
		reqs.Telemetry.NewCounter(moduleName, "invalid_ip_address", []string{}, "Counter measuring the number of rDNS requests with an invalid IP address"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_not_found", []string{}, "Counter measuring the number of rDNS lookups that returned a not found error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_timeout", []string{}, "Counter measuring the number of rDNS lookups that returned a timeout error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_temporary", []string{}, "Counter measuring the number of rDNS lookups that returned a temporary error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_other", []string{}, "Counter measuring the number of rDNS lookups that returned error not otherwise classified"),
		reqs.Telemetry.NewCounter(moduleName, "successful", []string{}, "Counter measuring the number of successful rDNS requests"),
	}

	q := &rdnsQuerierImpl{
		logger:            reqs.Logger,
		config:            config,
		internalTelemetry: internalTelemetry,

		started:     false,
		resolver:    newResolver(config),
		rateLimiter: newRateLimiter(config),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: q.start,
		OnStop:  q.stop,
	})

	return Provides{
		Comp: q,
	}, nil
}

// GetHostnameAsync attempts to resolve the hostname for the given IP address.
// If the IP address is invalid then an error is returned.
// If the IP address is not in the private address space then it is ignored - no lookup is performed and no error is returned.
// If the IP address is in the private address space then a reverse DNS lookup request is sent to a channel to be processed asynchronously.
// If the channel is full then an error is returned.
// When the lookup request completes the updateHostname function will be called asynchronously with the results.
func (q *rdnsQuerierImpl) GetHostnameAsync(ipAddr []byte, updateHostname func(string)) error {
	q.internalTelemetry.total.Inc()

	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		q.internalTelemetry.invalidIPAddress.Inc()
		return fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !ipaddr.IsPrivate() {
		q.logger.Tracef("Reverse DNS Enrichment IP address %s is not in the private address space", ipaddr)
		return nil
	}
	q.internalTelemetry.private.Inc()

	query := &rdnsQuery{
		addr:           ipaddr.String(),
		updateHostname: updateHostname,
	}

	select {
	case q.rdnsQueryChan <- query:
		q.internalTelemetry.chanAdded.Inc()
	default:
		q.internalTelemetry.droppedChanFull.Inc()
		return fmt.Errorf("channel is full, dropping query for IP address %s", query.addr)
	}
	return nil
}

func (q *rdnsQuerierImpl) start(_ context.Context) error {
	if q.started {
		q.logger.Debugf("Reverse DNS Enrichment already started")
		return nil
	}

	// A context is needed by the rate limiter and we also use its Done() channel for shutting down worker goroutines.
	// We don't use the context passed in because it has a deadline set, which we don't want.
	var ctx context.Context
	ctx, q.cancel = context.WithCancel(context.Background())

	q.rdnsQueryChan = make(chan *rdnsQuery, q.config.chanSize)

	for range q.config.workers {
		q.wg.Add(1)
		go q.worker(ctx)
	}
	q.logger.Infof("Reverse DNS Enrichment started %d rdnsquerier workers", q.config.workers)
	q.started = true

	return nil
}

func (q *rdnsQuerierImpl) stop(context.Context) error {
	if !q.started {
		q.logger.Debugf("Reverse DNS Enrichment already stopped")
		return nil
	}

	q.cancel()
	q.wg.Wait()

	q.logger.Infof("Reverse DNS Enrichment stopped rdnsquerier workers")
	q.started = false

	return nil
}

func (q *rdnsQuerierImpl) worker(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case query := <-q.rdnsQueryChan:
			q.getHostname(ctx, query)
		case <-ctx.Done():
			return
		}
	}
}

func (q *rdnsQuerierImpl) getHostname(ctx context.Context, query *rdnsQuery) {
	err := q.rateLimiter.wait(ctx)
	if err != nil {
		q.internalTelemetry.droppedRateLimiter.Inc()
		q.logger.Debugf("Reverse DNS Enrichment rateLimiter.wait() returned error: %v - dropping query for IP address %s", err, query.addr)
		return
	}

	hostname, err := q.resolver.lookup(query.addr)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok {
			if dnsErr.IsNotFound {
				q.internalTelemetry.lookupErrNotFound.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned not found error '%v' for IP address %v", err, query.addr)
				// no match was found for the requested IP address, so call updateHostname() to make the caller aware of that fact
				query.updateHostname(hostname)
				return
			}
			if dnsErr.IsTimeout {
				q.internalTelemetry.lookupErrTimeout.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned timeout error '%v' for IP address %v", err, query.addr)
				return
			}
			if dnsErr.IsTemporary {
				q.internalTelemetry.lookupErrTemporary.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned temporary error '%v' for IP address %v", err, query.addr)
				return
			}
		}
		q.internalTelemetry.lookupErrOther.Inc()
		q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned error '%v' for IP address %v", err, query.addr)
		return
	}

	q.internalTelemetry.successful.Inc()
	query.updateHostname(hostname)
}
