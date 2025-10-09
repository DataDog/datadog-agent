// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connfiltertype define config types for connfilter
package connfiltertype

// FilterType is the filter type struct
type FilterType string

const (
	// FilterTypeInclude const for include
	FilterTypeInclude FilterType = "include"
	// FilterTypeExclude const for exclude
	FilterTypeExclude FilterType = "exclude"
)

// MatchDomainStrategyType type for match domain strategy
type MatchDomainStrategyType string

const (
	// MatchDomainStrategyWildcard const for wildcard
	MatchDomainStrategyWildcard MatchDomainStrategyType = "wildcard"
	// MatchDomainStrategyRegex const for regex
	MatchDomainStrategyRegex MatchDomainStrategyType = "regex"
)

// Config represent one filter
type Config struct {
	Type                FilterType              `mapstructure:"type"`
	MatchDomain         string                  `mapstructure:"match_domain"`
	MatchDomainStrategy MatchDomainStrategyType `mapstructure:"match_domain_strategy"`
	MatchIP             string                  `mapstructure:"match_ip"`
}
