// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package filters

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traces"
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
func (f Replacer) Replace(trace traces.Trace) {
	for _, rule := range f.rules {
		key, str, re := rule.Name, rule.Repl, rule.Re
		for _, s := range trace.Spans {
			switch key {
			case "*":
				s.ForEachMetaUnsafe(func(k, v string) bool {
					s.SetMeta(k, re.ReplaceAllString(v, str))
					return true
				})
				s.SetResource(re.ReplaceAllString(s.UnsafeResource(), str))
			case "resource.name":
				s.SetResource(re.ReplaceAllString(s.UnsafeResource(), str))
			default:
				v, ok := s.GetMetaUnsafe(key)
				if !ok {
					continue
				}
				s.SetMeta(key, re.ReplaceAllString(v, str))
			}
		}
	}
}
