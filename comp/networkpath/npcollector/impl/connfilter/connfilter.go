// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connfilter manages connection filter configurations
package connfilter

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
)

// Filter represent one filter
type Filter struct {
	Type FilterType

	// TestConfigID preserves RC filter provenance so a Dynamic Test payload can
	// identify the remote configuration responsible for admitting its path. It
	// is empty for built-in and local filters.
	TestConfigID string
	matchDomain  *regexp.Regexp
	matchIPCidr  netip.Prefix
}

// ConnFilter class
type ConnFilter struct {
	filters []Filter
}

// NewConnFilter constructor
func NewConnFilter(config []Config, site string, monitorIPWithoutDomain bool) (*ConnFilter, []error) {
	defaultConfig := getDefaultConnFilters(site, monitorIPWithoutDomain)
	newConfigs := append(defaultConfig, config...)

	var filters []Filter
	var errs []error
	for _, cfg := range newConfigs {
		if cfg.Type != FilterTypeInclude && cfg.Type != FilterTypeExclude {
			errs = append(errs, fmt.Errorf("invalid filter type: %s", cfg.Type))
			continue
		}
		var matchDomainRe *regexp.Regexp
		var matchIPCidr netip.Prefix
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
			ip, err := netip.ParseAddr(cfg.MatchIP)
			if err == nil { // cfg.MatchIP is a single IP
				cidrStr = cfg.MatchIP + "/" + strconv.Itoa(ip.BitLen())
			} else { // assuming cfg.MatchIP is a CIDR
				cidrStr = cfg.MatchIP
			}
			cidr, err := netip.ParsePrefix(cidrStr)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to parsing match_ip `%s`: %s", cfg.MatchIP, err))
				continue
			}
			matchIPCidr = cidr
		}

		filters = append(filters, Filter{
			Type:         cfg.Type,
			TestConfigID: cfg.TestConfigID,
			matchDomain:  matchDomainRe,
			matchIPCidr:  matchIPCidr,
		})
	}
	return &ConnFilter{
		filters: filters,
	}, errs
}

// IsIncluded return true if the matching domain and ip of a connection should be included
func (f *ConnFilter) IsIncluded(domain string, ip netip.Addr) bool {
	isIncluded, _ := f.Evaluate(domain, ip)
	return isIncluded
}

// Evaluate returns whether a connection is included and the test config ID of
// the winning rule. Local and built-in rules have no test config ID.
func (f *ConnFilter) Evaluate(domain string, ip netip.Addr) (bool, string) {
	isIncluded := true
	testConfigID := ""
	if domain == "" {
		isIncluded = false
	}
	for _, filter := range f.filters {
		matched := false
		if filter.matchDomain != nil {
			if filter.matchDomain.MatchString(domain) {
				matched = true
			}
		}
		if filter.matchIPCidr.IsValid() && ip.IsValid() {
			if filter.matchIPCidr.Contains(ip) {
				matched = true
			}
		}
		if matched {
			testConfigID = filter.TestConfigID
			if filter.Type == FilterTypeExclude {
				isIncluded = false
			} else {
				isIncluded = true
			}
		}
	}
	if !isIncluded {
		return false, ""
	}
	return true, testConfigID
}
