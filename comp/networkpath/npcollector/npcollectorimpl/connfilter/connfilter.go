package connfilter

import (
	"fmt"
	"net"
	"regexp"
)

type Type string

const (
	filterTypeInclude Type = "include"
	filterTypeExclude Type = "exclude"
)

type matchDomainStrategyType string

const (
	matchDomainStrategyWildcard matchDomainStrategyType = "wildcard"
	matchDomainStrategyRegex    matchDomainStrategyType = "regex"
)

// Config represent one filter
type Config struct {
	Type                Type                    `mapstructure:"type"`
	MatchDomain         string                  `mapstructure:"match_domain"`
	MatchDomainStrategy matchDomainStrategyType `mapstructure:"match_domain_strategy"`
	MatchIP             string                  `mapstructure:"match_ip"`
}

// Filter represent one filter
type Filter struct {
	Type        Type
	matchDomain *regexp.Regexp
	matchIPCidr *net.IPNet
}

// ConnFilter class
type ConnFilter struct {
	filters []Filter
}

// network_path:
//  collector:
//    filter:
//      - match_domain: '*.datadoghq.com'
//        type: include
//      - match_domain: '*.google.com'
//        type: include
//      - match_ip: <IP or CIDR>
//        type: include
//      - match_domain: '*.zoom.us'
//        match_domain_strategy: wildcard                 # wildcard (default) | regex
//        type: exclude
//      - match_domain: '.*\.zoom\.us'
//        match_domain_strategy: regex
//        type: exclude
//      - match_ip: <IP or CIDR>
//        # match_port: <port>                            # add later if user ask for it
//        # match_protocol: <TCP | UDP | ICMP>            # add later if user ask for it
//        type: exclude

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
