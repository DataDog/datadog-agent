// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package filters

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// Replacer is a filter which replaces tag values based on its
// settings. It keeps all spans.
type Replacer struct {
	rules []*config.ReplaceRule
}

// NewReplacer returns a new Replacer which will use the given set of rules.
func NewReplacer(rules []*config.ReplaceRule) *Replacer {
	return &Replacer{rules: rules}
}

// Replace replaces all tags matching the Replacer's rules.
func (f Replacer) Replace(trace pb.Trace) {
	for _, rule := range f.rules {
		key, str, re := rule.Name, rule.Repl, rule.Re
		for _, s := range trace {
			switch key {
			case "*":
				for k := range s.Meta {
					s.Meta[k] = re.ReplaceAllString(s.Meta[k], str)
				}
				s.Resource = re.ReplaceAllString(s.Resource, str)
			case "resource.name":
				s.Resource = re.ReplaceAllString(s.Resource, str)
			default:
				if s.Meta == nil {
					continue
				}
				if _, ok := s.Meta[key]; !ok {
					continue
				}
				s.Meta[key] = re.ReplaceAllString(s.Meta[key], str)
			}
		}
	}
}
