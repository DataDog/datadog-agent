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
	"slices"
	"strconv"
)

// Filter represent one filter
type Filter struct {
	Type        FilterType
	matchDomain *regexp.Regexp
	matchIPCidr netip.Prefix

	// TestConfigID preserves RC filter provenance so a Dynamic Test payload can
	// identify the remote configuration responsible for admitting its path. It
	// is empty for built-in and local filters.
	TestConfigID string
	Tags         []string
}

// matches reports whether the filter matches the given domain and/or IP.
func (filter *Filter) matches(domain string, ip netip.Addr) bool {
	if filter.matchDomain != nil && filter.matchDomain.MatchString(domain) {
		return true
	}
	if filter.matchIPCidr.IsValid() && ip.IsValid() && filter.matchIPCidr.Contains(ip) {
		return true
	}
	return false
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
			Type:        cfg.Type,
			matchDomain: matchDomainRe,
			matchIPCidr: matchIPCidr,

			TestConfigID: cfg.TestConfigID,
			Tags:         slices.Clone(cfg.Tags),
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
	included, testConfigID, _ := f.EvaluateWithTags(domain, ip)
	return included, testConfigID
}

// EvaluateWithTags also returns the config tags of the winning rule. The
// returned tags are owned by the filter and must not be modified.
func (f *ConnFilter) EvaluateWithTags(domain string, ip netip.Addr) (bool, string, []string) {
	isIncluded := true
	testConfigID := ""
	var tags []string
	if domain == "" {
		isIncluded = false
	}
	for _, filter := range f.filters {
		if filter.matches(domain, ip) {
			testConfigID = filter.TestConfigID
			tags = filter.Tags
			if filter.Type == FilterTypeExclude {
				isIncluded = false
			} else {
				isIncluded = true
			}
		}
	}
	if !isIncluded {
		return false, "", nil
	}
	return true, testConfigID, tags
}

// EvaluateDomains evaluates the filter across every DNS name mapped to a
// destination IP, rather than a single pre-selected name. It exists because a
// destination IP can be reverse-resolved to more than one name (e.g. a Datadog
// intake endpoint CNAME'd to an AWS ELB, where both the datadoghq.com name and
// the ELB name are cached for the same IP). Evaluating only the first name lets
// Datadog infrastructure slip past the default excludes depending on cache
// ordering.
//
// Semantics:
//   - Every name is evaluated through the full filter chain. Because the chain
//     is last-match-wins, a customer include rule still overrides a default
//     exclude for a given name — customers can re-enable Datadog domains.
//   - The connection is excluded if any name evaluates to excluded. This is what
//     closes the CNAME gap: a name that resolves to Datadog intake excludes the
//     connection even when another cached name for the same IP does not.
//   - When included, the returned hostname is the preferred name for the path
//     test: a name admitted by an include rule if any, else the first included
//     name.
func (f *ConnFilter) EvaluateDomains(domains []string, ip netip.Addr) (bool, string, string, []string) {
	if len(domains) == 0 {
		domains = []string{""}
	}

	firstIncluded := -1
	includeMatched := -1
	var testConfigID string
	var tags []string
	for i, domain := range domains {
		included, id, t := f.EvaluateWithTags(domain, ip)
		if !included {
			// Any excluded name excludes the whole connection. A customer
			// include rule flips a name back to included (last-match-wins in
			// EvaluateWithTags), which is how defaults are overridden.
			return false, "", "", nil
		}
		if firstIncluded == -1 {
			firstIncluded, testConfigID, tags = i, id, t
		}
		// A non-empty TestConfigID means an RC include rule admitted this name;
		// prefer it as the path test hostname.
		if id != "" && includeMatched == -1 {
			includeMatched, testConfigID, tags = i, id, t
		}
	}

	if firstIncluded == -1 {
		return false, "", "", nil
	}
	hostname := domains[firstIncluded]
	if includeMatched != -1 {
		hostname = domains[includeMatched]
	}
	return true, hostname, testConfigID, tags
}
