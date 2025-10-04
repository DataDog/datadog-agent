package filter

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

// Filter class
type Filter struct {
	config []Config
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

func NewFilter(config []Config) Filter {
	return Filter{
		config: config,
	}
}
