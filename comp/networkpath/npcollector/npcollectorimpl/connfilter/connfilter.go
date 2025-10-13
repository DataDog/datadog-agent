// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connfilter manages connection filter configurations
package connfilter

import (
	"fmt"
	"net"
	"regexp"
)

// Filter represent one filter
type Filter struct {
	Type        FilterType
	matchDomain *regexp.Regexp
	matchIPCidr *net.IPNet
}

// ConnFilter class
type ConnFilter struct {
	filters []Filter
}

// NewConnFilter constructor
func NewConnFilter(config []Config, site string) (*ConnFilter, []error) {
	defaultConfig := getDefaultConnFilters(site)
	newConfigs := append(defaultConfig, config...)

	var filters []Filter
	var errs []error
	for _, cfg := range newConfigs {
		if cfg.Type != FilterTypeInclude && cfg.Type != FilterTypeExclude {
			errs = append(errs, fmt.Errorf("invalid filter type: %s", cfg.Type))
		}
		var matchDomainRe *regexp.Regexp
		var matchIPCidr *net.IPNet
		if cfg.MatchDomain != "" {
			matchDomainStrat := cfg.MatchDomainStrategy
			if matchDomainStrat == "" {
				matchDomainStrat = MatchDomainStrategyWildcard
			}
			if matchDomainStrat != MatchDomainStrategyWildcard && matchDomainStrat != MatchDomainStrategyRegex {
				errs = append(errs, fmt.Errorf("invalid match domain strategy: %s", matchDomainStrat))
				continue
			}
			domainRe, err := buildRegex(cfg.MatchDomain, matchDomainStrat)
			if err != nil {
				errs = append(errs, fmt.Errorf("error building regex `%s`: %s", cfg.MatchDomain, err))
				continue
			}
			matchDomainRe = domainRe
		}
		if cfg.MatchIP != "" {
			var cidrStr string
			ip := net.ParseIP(cfg.MatchIP)
			if ip != nil { // cfg.MatchIP is a single IP
				if ip.To4() != nil {
					cidrStr = cfg.MatchIP + "/32"
				} else if ip.To16() != nil {
					cidrStr = cfg.MatchIP + "/128"
				}
			} else { // assuming cfg.MatchIP is a CIDR
				cidrStr = cfg.MatchIP
			}
			_, cidr, err := net.ParseCIDR(cidrStr)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to parsing match_ip `%s`: %s", cfg.MatchIP, err))
			}
			matchIPCidr = cidr
		}

		filters = append(filters, Filter{
			Type:        cfg.Type,
			matchDomain: matchDomainRe,
			matchIPCidr: matchIPCidr,
		})
	}
	return &ConnFilter{
		filters: filters,
	}, errs
}

// IsIncluded return true if the matching domain and ip of a connection should be included
func (f *ConnFilter) IsIncluded(domain string, ip string) bool {
	isIncluded := true
	for _, filter := range f.filters {
		matched := false
		if filter.matchDomain != nil {
			if filter.matchDomain.MatchString(domain) {
				matched = true
			}
		}
		if filter.matchIPCidr != nil {
			if filter.matchIPCidr.Contains(net.ParseIP(ip)) {
				matched = true
			}
		}
		if matched {
			if filter.Type == FilterTypeExclude {
				isIncluded = false
			} else {
				isIncluded = true
			}
		}
	}
	return isIncluded
}
