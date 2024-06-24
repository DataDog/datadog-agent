// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the rdnsquerier component interface
package rdnsquerierimpl

import (
	"net"
	"net/netip"

	"context"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierimplnone "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
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

type rdnsQuerierImpl struct {
	config config.Component
	logger log.Component
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
		config: reqs.Config,
		logger: reqs.Logger,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: q.start,
		OnStop:  q.stop,
	})

	return Provides{
		Comp: q,
	}, nil
}

func (q *rdnsQuerierImpl) start(context.Context) error {
	// TODO start workers
	return nil
}

func (q *rdnsQuerierImpl) stop(context.Context) error {
	// TODO stop workers
	return nil
}

// GetHostname attempts to use reverse DNS lookup to resolve the hostname for the given IP address.
// If the IP address is in the private address space and the lookup is successful it calls the
// updateHostname function with the hostname.
func (q *rdnsQuerierImpl) GetHostname(ipAddr []byte, updateHostname func(string)) {
	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		// IP address is invalid
		return
	}

	if !ipaddr.IsPrivate() {
		return
	}

	addr := ipaddr.String()

	go func() {
		// net.LookupAddr() can return both a non-zero length slice of hostnames and an error, but when
		// using the host C library resolver at most one result will be returned.  So for now, since
		// specifying other DNS resolvers is not supported, if we get an error we know that no valid
		// hostname was returned.
		hostnames, err := net.LookupAddr(addr)
		if err != nil || len(hostnames) == 0 {
			return
		}

		updateHostname(hostnames[0])
	}()
}
