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

// FilterType is the filter type struct
type FilterType string

const (
	filterTypeInclude FilterType = "include"
	filterTypeExclude FilterType = "exclude"
)

type matchDomainStrategyType string

const (
	matchDomainStrategyWildcard matchDomainStrategyType = "wildcard"
	matchDomainStrategyRegex    matchDomainStrategyType = "regex"
)

// Config represent one filter
type Config struct {
	Type                FilterType              `mapstructure:"type"`
	MatchDomain         string                  `mapstructure:"match_domain"`
	MatchDomainStrategy matchDomainStrategyType `mapstructure:"match_domain_strategy"`
	MatchIP             string                  `mapstructure:"match_ip"`
}

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
	// TODO: test compile error
	defaultConfig := getDefaultConnFilters(site)
	newConfigs := append(defaultConfig, config...)

	var filters []Filter
	var errs []error
	for _, cfg := range newConfigs {
		var matchDomainRe *regexp.Regexp
		var matchIPCidr *net.IPNet
		if cfg.MatchDomain != "" {
			matchDomainStrat := cfg.MatchDomainStrategy
			if matchDomainStrat == "" {
				// TODO: TEST ME
				matchDomainStrat = matchDomainStrategyWildcard
			}
			if matchDomainStrat != matchDomainStrategyWildcard && matchDomainStrat != matchDomainStrategyRegex {
				errs = append(errs, fmt.Errorf("invalid match domain strategy: %s", matchDomainStrat))
				// TODO: TEST ME
				continue
			}
			domainRe, err := buildRegex(cfg.MatchDomain, matchDomainStrat)
			if err != nil {
				// TODO: TEST ME
				errs = append(errs, fmt.Errorf("error building regex `%s`: %s", cfg.MatchDomain, err))
				continue
			}
			matchDomainRe = domainRe
		}
		if cfg.MatchIP != "" {
			var cidrStr string
			ip := net.ParseIP(cfg.MatchIP)
			if ip != nil { // TODO: Does this work?
				if ip.To4() != nil {
					// TODO: TEST ME
					cidrStr = cfg.MatchIP + "/32"
				} else if ip.To16() != nil {
					// TODO: TEST ME
					cidrStr = cfg.MatchIP + "/128"
				}
			} else {
				cidrStr = cfg.MatchIP
			}
			_, cidr, err := net.ParseCIDR(cidrStr)
			if err != nil {
				// TODO: TEST ME
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
		if filter.matchDomain != nil { // TODO: TEST ME
			if filter.matchDomain.MatchString(domain) {
				matched = true
			}
		}
		if filter.matchIPCidr != nil { // TODO: TEST ME
			if filter.matchIPCidr.Contains(net.ParseIP(ip)) {
				matched = true
			}
		}
		if matched {
			if filter.Type == filterTypeExclude {
				isIncluded = false
			} else {
				isIncluded = true
			}
		}
	}
	return isIncluded
}
