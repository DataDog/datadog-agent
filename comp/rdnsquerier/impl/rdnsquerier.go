// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the rdnsquerier component interface
package rdnsquerierimpl

import (
	"net"
	"net/netip"
	"sync"

	"context"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierimplnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
)

// TODO add config
const (
	numWorkers    = 10
	queryChanSize = 1000
)

// Requires defines the dependencies for the rdnsquerier component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

// Provides defines the output of the rdnsquerier component
type Provides struct {
	Comp rdnsquerier.Component
}

type rdnsQuery struct {
	addr           string
	updateHostname func(string)
}

type rdnsQuerierImpl struct {
	config config.Component
	logger log.Component

	rdnsQueryChan chan *rdnsQuery
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewComponent creates a new rdnsquerier component
func NewComponent(reqs Requires) (Provides, error) {
	netflowRDNSEnrichmentEnabled := reqs.Config.GetBool("network_devices.netflow.reverse_dns_enrichment_enabled")

	if !netflowRDNSEnrichmentEnabled {
		return Provides{
			Comp: rdnsquerierimplnone.NewNone().Comp,
		}, nil
	}

	q := &rdnsQuerierImpl{
		config:        reqs.Config,
		logger:        reqs.Logger,
		rdnsQueryChan: make(chan *rdnsQuery, queryChanSize),
		stopChan:      make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: q.start,
		OnStop:  q.stop,
	})

	return Provides{
		Comp: q,
	}, nil
}

// GetHostname attempts to use reverse DNS lookup to resolve the hostname for the given IP address.
// If the IP address is in the private address space and the lookup is successful then the updateHostname
// function will be called with the hostname.
func (q *rdnsQuerierImpl) GetHostname(ipAddr []byte, updateHostname func(string)) {
	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		// IP address is invalid
		return
	}

	if !ipaddr.IsPrivate() {
		return
	}

	q.rdnsQueryChan <- &rdnsQuery{
		addr:           ipaddr.String(),
		updateHostname: updateHostname,
	}
}

func (q *rdnsQuerierImpl) start(context.Context) error {
	for i := 0; i < numWorkers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	q.logger.Tracef("Started %d rdnsquerier workers", numWorkers)

	return nil
}

func (q *rdnsQuerierImpl) stop(context.Context) error {
	close(q.stopChan)
	q.wg.Wait()
	q.logger.Infof("Stopped rdnsquerier workers")

	return nil
}

func (q *rdnsQuerierImpl) worker(num int) {
	defer q.wg.Done()
	for {
		select {
		case query := <-q.rdnsQueryChan:
			q.logger.Tracef("worker[%d] processing rdnsQuery for IP address %v", num, query.addr)
			q.getHostname(query)
		case <-q.stopChan:
			return
		}
	}
}

func (q *rdnsQuerierImpl) getHostname(query *rdnsQuery) {
	// net.LookupAddr() can return both a non-zero length slice of hostnames and an error, but when
	// using the host C library resolver at most one result will be returned.  So for now, since
	// specifying other DNS resolvers is not supported, if we get an error we know that no valid
	// hostname was returned.
	hostnames, err := net.LookupAddr(query.addr)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok {
			if dnsErr.IsNotFound {
				q.logger.Tracef("net.LookupAddr returned not found error '%v' for IP address %v", err, query.addr)
				return
			}
			if dnsErr.IsTimeout {
				q.logger.Tracef("net.LookupAddr returned timeout error '%v' for IP address %v", err, query.addr)
				return
			}
			if dnsErr.IsTemporary {
				q.logger.Tracef("net.LookupAddr returned temporary error '%v' for IP address %v", err, query.addr)
				return
			}
		}
		q.logger.Tracef("net.LookupAddr returned unknown error '%v' for IP address %v", err, query.addr)
		return
	}

	if len(hostnames) > 0 { // if !err then there should be at least one, but just to be safe
		query.updateHostname(hostnames[0])
	}
}
