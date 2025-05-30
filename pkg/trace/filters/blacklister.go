// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filters provides a way to filter out spans based on their properties.
package filters

import (
	"regexp"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// Blacklister holds a list of regular expressions which will match resources
// on spans that should be dropped.
type Blacklister struct {
	list []*regexp.Regexp
}

// Allows returns true if the Blacklister permits this span. False and the denying rule otherwise.
func (f *Blacklister) Allows(span *pb.Span) (bool, *regexp.Regexp) {
	for _, entry := range f.list {
		if entry.MatchString(span.Resource) {
			return false, entry
		}
	}
	return true, nil
}

// AllowsStat returns true if the Blacklister permits this stat
func (f *Blacklister) AllowsStat(stat *pb.ClientGroupedStats) bool {
	for _, entry := range f.list {
		if entry.MatchString(stat.Resource) {
			return false
		}
	}
	return true
}

// NewBlacklister creates a new Blacklister based on the given list of
// regular expressions.
func NewBlacklister(exprs []string) *Blacklister {
	return &Blacklister{list: compileRules(exprs)}
}

// compileRules compiles as many rules as possible from the list of expressions.
func compileRules(exprs []string) []*regexp.Regexp {
	list := make([]*regexp.Regexp, 0, len(exprs))
	for _, entry := range exprs {
		rule, err := regexp.Compile(entry)
		if err != nil {
			log.Errorf("Invalid resource filter: %s: %s", entry, err)
			continue
		}
		list = append(list, rule)
	}
	return list
}
