// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the rdnsquerier component interface
package rdnsquerierimpl

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierimplnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
	"go.uber.org/multierr"
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

const moduleName = "reverse_dns_enrichment"

type rdnsQuerierTelemetry = struct {
	// rdnsquerier
	total            telemetry.Counter
	invalidIPAddress telemetry.Counter
	private          telemetry.Counter

	// cache
	cacheHit             telemetry.Counter
	cacheHitExpired      telemetry.Counter
	cacheHitInProgress   telemetry.Counter
	cacheMiss            telemetry.Counter
	cacheRetry           telemetry.Counter
	cacheRetriesExceeded telemetry.Counter
	cacheExpired         telemetry.Counter
	cacheSize            telemetry.Gauge
	cacheMaxSizeExceeded telemetry.Counter

	// querier
	chanAdded          telemetry.Counter
	droppedChanFull    telemetry.Counter
	droppedRateLimiter telemetry.Counter
	lookupErrNotFound  telemetry.Counter
	lookupErrTimeout   telemetry.Counter
	lookupErrTemporary telemetry.Counter
	lookupErrOther     telemetry.Counter
	successful         telemetry.Counter

	// rate limiter
	rateLimiterLimit telemetry.Gauge
}

type rdnsQuerierImpl struct {
	config            *rdnsQuerierConfig
	logger            log.Component
	internalTelemetry *rdnsQuerierTelemetry

	started bool

	cache cache
}

// NewComponent creates a new rdnsquerier component
func NewComponent(reqs Requires) (Provides, error) {
	rdnsQuerierConfig := newConfig(reqs.AgentConfig)
	reqs.Logger.Infof("Reverse DNS Enrichment config: (enabled=%t workers=%d chan_size=%d cache.enabled=%t cache.entry_ttl=%d cache.clean_interval=%d cache.persist_interval=%d cache.max_retries=%d cache.max_size=%d rate_limiter.enabled=%t rate_limiter.limit_per_sec=%d rate_limiter.limit_throttled_per_sec=%d rate_limiter.throttle_error_threshold=%d rate_limiter.recovery_intervals=%d rate_limiter.recovery_interval=%d)",
		rdnsQuerierConfig.enabled,
		rdnsQuerierConfig.workers,
		rdnsQuerierConfig.chanSize,

		rdnsQuerierConfig.cache.enabled,
		rdnsQuerierConfig.cache.entryTTL,
		rdnsQuerierConfig.cache.cleanInterval,
		rdnsQuerierConfig.cache.persistInterval,
		rdnsQuerierConfig.cache.maxRetries,
		rdnsQuerierConfig.cache.maxSize,

		rdnsQuerierConfig.rateLimiter.enabled,
		rdnsQuerierConfig.rateLimiter.limitPerSec,
		rdnsQuerierConfig.rateLimiter.limitThrottledPerSec,
		rdnsQuerierConfig.rateLimiter.throttleErrorThreshold,
		rdnsQuerierConfig.rateLimiter.recoveryIntervals,
		rdnsQuerierConfig.rateLimiter.recoveryInterval,
	)

	if !rdnsQuerierConfig.enabled {
		return Provides{
			Comp: rdnsquerierimplnone.NewNone().Comp,
		}, nil
	}

	internalTelemetry := &rdnsQuerierTelemetry{
		reqs.Telemetry.NewCounter(moduleName, "total", []string{}, "Counter measuring the total number of rDNS requests"),
		reqs.Telemetry.NewCounter(moduleName, "invalid_ip_address", []string{}, "Counter measuring the number of rDNS requests with an invalid IP address"),
		reqs.Telemetry.NewCounter(moduleName, "private", []string{}, "Counter measuring the number of rDNS requests in the private address space"),

		reqs.Telemetry.NewCounter(moduleName, "cache_hit", []string{}, "Counter measuring the number of successful rDNS cache hits"),
		reqs.Telemetry.NewCounter(moduleName, "cache_hit_expired", []string{}, "Counter measuring the number of expired rDNS cache hits"),
		reqs.Telemetry.NewCounter(moduleName, "cache_hit_in_progress", []string{}, "Counter measuring the number of in progress rDNS cache hits"),
		reqs.Telemetry.NewCounter(moduleName, "cache_miss", []string{}, "Counter measuring the number of rDNS cache misses"),
		reqs.Telemetry.NewCounter(moduleName, "cache_retry", []string{}, "Counter measuring the number of rDNS lookup retries from the rDNS cache"),
		reqs.Telemetry.NewCounter(moduleName, "cache_retries_exceeded", []string{}, "Counter measuring the number of times the number of rDNS lookup retries exceeded the maximum number of retries"),
		reqs.Telemetry.NewCounter(moduleName, "cache_expired", []string{}, "Counter measuring the number of expired rDNS cache entries"),
		reqs.Telemetry.NewGauge(moduleName, "cache_size", []string{}, "Gauge measuring the number of rDNS cache entries"),
		reqs.Telemetry.NewCounter(moduleName, "cache_max_size_exceeded", []string{}, "Counter measuring the number of times the rDNS cache exceeded the maximum size"),

		reqs.Telemetry.NewCounter(moduleName, "chan_added", []string{}, "Counter measuring the number of rDNS requests added to the channel"),
		reqs.Telemetry.NewCounter(moduleName, "dropped_chan_full", []string{}, "Counter measuring the number of rDNS requests dropped because the channel was full"),
		reqs.Telemetry.NewCounter(moduleName, "dropped_rate_limiter", []string{}, "Counter measuring the number of rDNS requests dropped because the rate limiter wait failed"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_not_found", []string{}, "Counter measuring the number of rDNS lookups that returned a not found error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_timeout", []string{}, "Counter measuring the number of rDNS lookups that returned a timeout error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_temporary", []string{}, "Counter measuring the number of rDNS lookups that returned a temporary error"),
		reqs.Telemetry.NewCounter(moduleName, "lookup_err_other", []string{}, "Counter measuring the number of rDNS lookups that returned error not otherwise classified"),
		reqs.Telemetry.NewCounter(moduleName, "successful", []string{}, "Counter measuring the number of successful rDNS requests"),

		reqs.Telemetry.NewGauge(moduleName, "rate_limiter_limit", []string{}, "Gauge measuring the rDNS rate limiter limit per second"),
	}

	q := &rdnsQuerierImpl{
		config:            rdnsQuerierConfig,
		logger:            reqs.Logger,
		internalTelemetry: internalTelemetry,

		started: false,
		cache:   newCache(rdnsQuerierConfig, reqs.Logger, internalTelemetry, newQuerier(rdnsQuerierConfig, reqs.Logger, internalTelemetry)),
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
// If the IP address is not in the private address space then it is ignored - no lookup is performed and nil error is returned.
// If the IP address is in the private address space then the IP address will be resolved to a hostname.
// If the hostname for the IP address is immediately available (i.e. cache is enabled and entry is cached) then the updateHostnameSync callback
// will be invoked synchronously, otherwise a query is sent to a channel to be processed asynchronously.  If the channel is full then an error
// is returned.  When the request completes the updateHostnameAsync callback will be invoked asynchronously.
func (q *rdnsQuerierImpl) GetHostnameAsync(ipAddr []byte, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
	q.internalTelemetry.total.Inc()

	netipAddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		q.internalTelemetry.invalidIPAddress.Inc()
		return fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !netipAddr.IsPrivate() {
		q.logger.Tracef("Reverse DNS Enrichment IP address %s is not in the private address space", ipAddr)
		return nil
	}
	q.internalTelemetry.private.Inc()

	err := q.cache.getHostname(netipAddr.String(), updateHostnameSync, updateHostnameAsync)
	if err != nil {
		q.logger.Debugf("Reverse DNS Enrichment cache.getHostname() for addr %s returned error: %v", netipAddr.String(), err)
	}

	return err
}

// GetHostname attempts to resolve the hostname for the given IP address synchronously.
// If the IP address is invalid then an error is returned.
// If the IP address is not in the private address space then it is ignored - no lookup is performed and nil error is returned.
// If the IP address is in the private address space then the IP address will be resolved to a hostname.
// The function accepts a timeout via context and will return an error if the timeout is reached.
func (q *rdnsQuerierImpl) GetHostname(ctx context.Context, ipAddr string) (string, error) {
	q.internalTelemetry.total.Inc()

	netipAddr, err := netip.ParseAddr(ipAddr)
	if err != nil {
		q.internalTelemetry.invalidIPAddress.Inc()
		return "", fmt.Errorf("invalid IP address %s: %v", ipAddr, err)
	}

	if !netipAddr.IsPrivate() {
		q.logger.Tracef("Reverse DNS Enrichment IP address %s is not in the private address space", ipAddr)
		return "", nil
	}
	q.internalTelemetry.private.Inc()

	resultsChan := make(chan rdnsquerier.ReverseDNSResult, 1)

	err = q.cache.getHostname(
		netipAddr.String(),
		func(h string) {
			resultsChan <- rdnsquerier.ReverseDNSResult{Hostname: h}
		},
		func(h string, e error) {
			resultsChan <- rdnsquerier.ReverseDNSResult{Hostname: h, Err: e}
		},
	)
	if err != nil {
		q.logger.Debugf("Reverse DNS Enrichment cache.getHostname() for addr %s returned error: %v", netipAddr.String(), err)
		return "", err
	}

	select {
	case result := <-resultsChan:
		err = multierr.Append(err, result.Err)
		return result.Hostname, err
	case <-ctx.Done():
		return "", fmt.Errorf("timeout reached while resolving hostname for IP address %v", ipAddr)
	}
}

// GetHostnames attempts to resolve the hostname for the given IP addresses.
// If the IP address is invalid then an error is returned.
// If the IP address is not in the private address space then it is ignored - no lookup is performed and nil error is returned.
// If the IP address is in the private address space then the IP address will be resolved to a hostname.
// The function accepts a timeout via context and will return an error if the timeout is reached.
func (q *rdnsQuerierImpl) GetHostnames(ctx context.Context, ipAddrs []string) map[string]rdnsquerier.ReverseDNSResult {
	var wg sync.WaitGroup
	resultsChan := make(chan rdnsquerier.ReverseDNSResult, len(ipAddrs))

	for _, ipAddr := range ipAddrs {
		wg.Add(1)
		go func(ctx context.Context, ipAddr string) {
			defer wg.Done()
			hostname, err := q.GetHostname(ctx, ipAddr)
			resultsChan <- rdnsquerier.ReverseDNSResult{IP: ipAddr, Hostname: hostname, Err: err}
		}(ctx, ipAddr)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	results := make(map[string]rdnsquerier.ReverseDNSResult, len(ipAddrs))
	for result := range resultsChan {
		results[result.IP] = result
	}

	return results
}

func (q *rdnsQuerierImpl) start(_ context.Context) error {
	if q.started {
		q.logger.Debugf("Reverse DNS Enrichment already started")
		return nil
	}

	q.cache.start()
	q.started = true

	return nil
}

func (q *rdnsQuerierImpl) stop(context.Context) error {
	if !q.started {
		q.logger.Debugf("Reverse DNS Enrichment already stopped")
		return nil
	}

	q.cache.stop()
	q.started = false
	return nil
}
