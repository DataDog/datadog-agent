// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"context"
	"fmt"
	"net"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

type querier interface {
	start()
	stop()
	getHostnameAsync(addr string, updateHostnameAsync func(string, error)) error
}

// Standard querier implementation
type querierImpl struct {
	config            *rdnsQuerierConfig
	logger            log.Component
	internalTelemetry *rdnsQuerierTelemetry

	rateLimiter rateLimiter
	resolver    resolver

	// Context is used by the rate limiter and also for shutting down worker goroutines via its Done() channel.
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	queryChan chan *query
}

type query struct {
	addr                string
	updateHostnameAsync func(string, error)
}

func newQuerier(config *rdnsQuerierConfig, logger log.Component, internalTelemetry *rdnsQuerierTelemetry) querier {
	return &querierImpl{
		config:            config,
		logger:            logger,
		internalTelemetry: internalTelemetry,
		rateLimiter:       newRateLimiter(config, logger, internalTelemetry),
		resolver:          newResolver(config),
	}
}

func (q *querierImpl) start() {
	q.rateLimiter.start()

	q.ctx, q.cancel = context.WithCancel(context.Background())
	q.queryChan = make(chan *query, q.config.chanSize)

	for range q.config.workers {
		q.wg.Add(1)
		go q.worker()
	}
	q.logger.Infof("Reverse DNS Enrichment started %d workers", q.config.workers)
}

func (q *querierImpl) stop() {
	q.cancel()
	q.wg.Wait()
	q.logger.Infof("Reverse DNS Enrichment stopped workers")

	q.rateLimiter.stop()
}

func (q *querierImpl) worker() {
	defer q.wg.Done()
	for {
		select {
		case query := <-q.queryChan:
			q.getHostname(query)
		case <-q.ctx.Done():
			return
		}
	}
}

// getHostnameAsync attempts to resolve the hostname for the specified IP address.
// Resolution is handled asynchronously from the caller's perspective using worker goroutines.
// If the lookup is successful the updateHostnameAsync callback is invoked with hostname and nil error,
// otherwise it is invoked with hostname "" and the error that occurred.
// Note that an IsNotFound error is not treated as an error, but as a successful resolution with a hostname of "" and a nil error.
func (q *querierImpl) getHostnameAsync(addr string, updateHostnameAsync func(string, error)) error {
	select {
	case q.queryChan <- &query{addr: addr, updateHostnameAsync: updateHostnameAsync}:
		q.internalTelemetry.chanAdded.Inc()
		return nil
	default:
		q.internalTelemetry.droppedChanFull.Inc()
		return fmt.Errorf("channel is full, dropping query for IP address %s", addr)
	}
}

func (q *querierImpl) getHostname(query *query) {
	err := q.rateLimiter.wait(q.ctx)
	if err != nil {
		// note that this error should only occur during shutdown when rate limiter wait() calls are cancelled
		q.internalTelemetry.droppedRateLimiter.Inc()
		q.logger.Debugf("Reverse DNS Enrichment rateLimiter.wait() returned error: %v - dropping query for IP address %s", err, query.addr)
		query.updateHostnameAsync("", err)
		return
	}

	hostname, err := q.resolver.lookup(query.addr)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok {
			if dnsErr.IsNotFound {
				q.internalTelemetry.lookupErrNotFound.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned not found error '%v' for IP address %v", err, query.addr)
				// IsNotFound error is treated as a successful resolution with a hostname of ""
				query.updateHostnameAsync("", nil)
				q.rateLimiter.markSuccess()
				return
			}
			if dnsErr.IsTimeout {
				q.internalTelemetry.lookupErrTimeout.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned isTimeout error '%v' for IP address %v", err, query.addr)
			}
			if dnsErr.IsTemporary {
				q.internalTelemetry.lookupErrTemporary.Inc()
				q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned isTemporary error '%v' for IP address %v", err, query.addr)
			}
			query.updateHostnameAsync("", err)
			q.rateLimiter.markFailure()
			return
		}
		q.internalTelemetry.lookupErrOther.Inc()
		q.logger.Debugf("Reverse DNS Enrichment net.LookupAddr returned error '%v' for IP address %v", err, query.addr)
		query.updateHostnameAsync("", err)
		q.rateLimiter.markFailure()
		return
	}

	q.internalTelemetry.successful.Inc()
	q.logger.Tracef("Reverse DNS Enrichment q.resolver.lookup() successfully resolved IP address %s hostname = %s", query.addr, hostname)
	query.updateHostnameAsync(hostname, nil)
	q.rateLimiter.markSuccess()
}
