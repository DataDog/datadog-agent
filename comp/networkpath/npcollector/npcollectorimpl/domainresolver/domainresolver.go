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

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const domainLookupExpiration = 2 * time.Hour

// DomainResolver handles domain resolution
type DomainResolver struct {
	LookupHostFn func(host string) (addrs []string, err error)
}

// NewDomainResolver constructor
func NewDomainResolver() *DomainResolver {
	return &DomainResolver{
		LookupHostFn: net.LookupHost,
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
		ips, err := cache.GetWithExpiration(domain, func() ([]string, error) {
			ips, err := d.LookupHostFn(domain)
			return ips, err
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
