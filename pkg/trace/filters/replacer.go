// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
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

const hiddenTagPrefix = "_"

// ReplaceV1 replaces all tags matching the Replacer's rules.
func (f Replacer) ReplaceV1(trace *idx.InternalTraceChunk) {
	for _, rule := range f.rules {
		key, str, re := rule.Name, rule.Repl, rule.Re
		for _, s := range trace.Spans {
			switch key {
			case "*":
				for k, v := range s.Attributes() {
					kString := trace.Strings.Get(k)
					if strings.HasPrefix(kString, hiddenTagPrefix) {
						continue
					}
					vString := v.AsString(trace.Strings)
					newV := re.ReplaceAllString(vString, str)
					if newV != vString {
						s.SetAttributeFromString(kString, newV)
					}
				}
				s.SetResource(re.ReplaceAllString(s.Resource(), str))
				for _, spanEvent := range s.Events() {
					for keyAttr, val := range spanEvent.Attributes() {
						kString := trace.Strings.Get(keyAttr)
						if !strings.HasPrefix(kString, hiddenTagPrefix) {
							vString := val.AsString(trace.Strings)
							newV := re.ReplaceAllString(vString, str)
							if newV != vString {
								spanEvent.SetAttributeFromString(kString, newV)
							}
						}
					}
				}
			case "resource.name":
				s.SetResource(re.ReplaceAllString(s.Resource(), str))
			default:
				if val, ok := s.GetAttributeAsString(key); ok {
					s.SetAttributeFromString(key, re.ReplaceAllString(val, str))
				}
				for _, spanEvent := range s.Events() {
					if val, ok := spanEvent.GetAttributeAsString(key); ok {
						spanEvent.SetAttributeFromString(key, re.ReplaceAllString(val, str))
					}
				}
			}
		}
	}
}

// ReplaceStatsGroup applies the replacer rules to the given stats bucket group.
func (f Replacer) ReplaceStatsGroup(b *pb.ClientGroupedStats) {
	for _, rule := range f.rules {
		key, str, re := rule.Name, rule.Repl, rule.Re
		switch key {
		case "resource.name":
			b.Resource = re.ReplaceAllString(b.Resource, str)
		case "*":
			b.Resource = re.ReplaceAllString(b.Resource, str)
			fallthrough
		case "http.status_code":
			strcode := re.ReplaceAllString(strconv.FormatUint(uint64(b.HTTPStatusCode), 10), str)
			if code, err := strconv.ParseUint(strcode, 10, 32); err == nil {
				b.HTTPStatusCode = uint32(code)
			}
		}
	}
}
