// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package domainresolver manages domain related resolutions
package domainresolver

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const domainLookupExpiration = 1 * time.Hour

// DomainResolver handles domain resolution
type DomainResolver struct {
	LookupHostFn func(host string) (addrs []string, err error)
	statsdClient statsd.ClientInterface
}

// NewDomainResolver constructor
func NewDomainResolver(statsdClient statsd.ClientInterface) *DomainResolver {
	return &DomainResolver{
		LookupHostFn: net.LookupHost,
		statsdClient: statsdClient,
	}
}

// GetIPResolverForDomains returns an IP Resolver based on a list of domains
func (d *DomainResolver) GetIPResolverForDomains(domains []string) (*IPToDomainResolver, []error) {
	domainMap, errors := d.getIPToDomainMap(domains)
	return NewIPToDomainResolver(domainMap), errors
}

func (d *DomainResolver) getIPToDomainMap(domains []string) (map[string]string, []error) {
	var errList []error
	ipToDomain := make(map[string]string)
	for _, domain := range domains {
		ips, err := GetWithExpiration(domain, func() ([]string, error) {
			// TODO: REMOVE DOMAIN TAG
			// TODO: REMOVE DOMAIN TAG
			// TODO: REMOVE DOMAIN TAG
			_ = d.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"domain_resolver_calls", []string{}, 1)
			ips, err := d.LookupHostFn(domain)
			if err != nil {
				return nil, err
			}
			return ips, nil
		}, domainLookupExpiration)
		if err != nil {
			errList = append(errList, fmt.Errorf("error looking up IPs for domain %s: %s", domain, err))
			continue
		}
		for _, ip := range ips {
			ipToDomain[ip] = domain
		}
	}
	return ipToDomain, errList
}

// GetWithExpiration returns the value for 'key'.
// TODO: TEST ME
func GetWithExpiration[T any](key string, cb func() (T, error), expire time.Duration) (T, error) {
	if x, found := cache.Cache.Get(key); found {
		return x.(T), nil
	}

	res, err := cb()
	// We cache errors too
	cache.Cache.Set(key, res, expire)
	return res, err
}
