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
	matchIP     *net.IPNet
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

func NewConnFilter(config []Config) (*ConnFilter, []error) {
	// TODO: test compile error

	filters := make([]Filter, 0, len(config))
	var errs []error
	for _, cfg := range config {
		matchDomainRe, err := regexp.Compile(cfg.MatchDomain)
		if err != nil {
			errs = append(errs, fmt.Errorf("error compiling domain regex `%s`: %s", cfg.MatchDomain, err))
		}
		filters = append(filters, Filter{
			Type:        cfg.Type,
			matchDomain: matchDomainRe,
		})
	}
	return &ConnFilter{
		filters: filters,
	}, errs
}

func (f *ConnFilter) Match(domain string, ip string) bool {
	matched := true
	for _, filter := range f.filters {
		if filter.matchDomain.MatchString(domain) {
			if filter.Type == filterTypeExclude {
				matched = false
			} else {
				matched = true
			}
		}
	}
	//net.ParseCIDR
	//cidr := net.IPNet{}
	//cidr.Contains(net.ParseIP(domain))

	// domain included by default
	// ip included by default
	//for _, filters := range f.filters {
	//	//filters.
	//}
	return matched
}
