// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	"regexp"
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
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

// Replace replaces all tags matching the Replacer's rules.
func (f Replacer) Replace(trace pb.Trace) {
	for _, rule := range f.rules {
		key, str, re := rule.Name, rule.Repl, rule.Re
		for _, s := range trace {
			switch key {
			case "*":
				for k := range s.Meta {
					if !strings.HasPrefix(k, hiddenTagPrefix) {
						s.Meta[k] = re.ReplaceAllString(s.Meta[k], str)
					}
				}
				for k := range s.Metrics {
					if !strings.HasPrefix(k, hiddenTagPrefix) {
						f.replaceNumericTag(re, s, k, str)
					}
				}
				s.Resource = re.ReplaceAllString(s.Resource, str)
				for _, spanEvent := range s.SpanEvents {
					if spanEvent != nil {
						for keyAttr, val := range spanEvent.Attributes {
							if !strings.HasPrefix(keyAttr, hiddenTagPrefix) {
								spanEvent.Attributes[keyAttr] = f.replaceAttributeAnyValue(re, val, str)
							}
						}
					}
				}
			case "resource.name":
				s.Resource = re.ReplaceAllString(s.Resource, str)
			default:
				if s.Meta != nil {
					if _, ok := s.Meta[key]; ok {
						s.Meta[key] = re.ReplaceAllString(s.Meta[key], str)
					}
				}
				if s.Metrics != nil {
					if _, ok := s.Metrics[key]; ok {
						f.replaceNumericTag(re, s, key, str)
					}
				}
				for _, spanEvent := range s.SpanEvents {
					if spanEvent != nil {
						if val, ok := spanEvent.Attributes[key]; ok {
							spanEvent.Attributes[key] = f.replaceAttributeAnyValue(re, val, str)
						}
					}
				}
			}
		}
	}
}

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

// replaceNumericTag acts on the `metrics` portion of a span, if the resulting replacement is no longer a string the tag
// is moved to the `meta`.
func (f Replacer) replaceNumericTag(re *regexp.Regexp, s *pb.Span, key string, str string) {
	replacedValue := re.ReplaceAllString(strconv.FormatFloat(s.Metrics[key], 'f', -1, 64), str)
	if rf, err := strconv.ParseFloat(replacedValue, 64); err == nil {
		s.Metrics[key] = rf
	} else {
		s.Meta[key] = replacedValue
		delete(s.Metrics, key)
	}
}

func (f Replacer) replaceAttributeAnyValue(re *regexp.Regexp, val *pb.AttributeAnyValue, str string) *pb.AttributeAnyValue {
	switch val.Type {
	case pb.AttributeAnyValue_STRING_VALUE:
		return &pb.AttributeAnyValue{
			Type:        pb.AttributeAnyValue_STRING_VALUE,
			StringValue: re.ReplaceAllString(val.StringValue, str),
		}
	case pb.AttributeAnyValue_INT_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatInt(val.IntValue, 10), str)
		return attributeAnyValFromString(replacedValue)
	case pb.AttributeAnyValue_DOUBLE_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatFloat(val.DoubleValue, 'f', -1, 64), str)
		return attributeAnyValFromString(replacedValue)
	case pb.AttributeAnyValue_BOOL_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatBool(val.BoolValue), str)
		return attributeAnyValFromString(replacedValue)
	case pb.AttributeAnyValue_ARRAY_VALUE:
		for _, value := range val.ArrayValue.Values {
			*value = *f.replaceAttributeArrayValue(re, value, str) //todo test me
		}
		return val
	default:
		log.Error("Unknown OTEL AttributeAnyValue type %v, replacer code must be updated, replacing unknown type with `?`")
		return &pb.AttributeAnyValue{
			Type:        pb.AttributeAnyValue_STRING_VALUE,
			StringValue: "?",
		}
	}
}

func (f Replacer) replaceAttributeArrayValue(re *regexp.Regexp, val *pb.AttributeArrayValue, str string) *pb.AttributeArrayValue {
	switch val.Type {
	case pb.AttributeArrayValue_STRING_VALUE:
		return &pb.AttributeArrayValue{
			Type:        pb.AttributeArrayValue_STRING_VALUE,
			StringValue: re.ReplaceAllString(val.StringValue, str),
		}
	case pb.AttributeArrayValue_INT_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatInt(val.IntValue, 10), str)
		return attributeArrayValFromString(replacedValue)
	case pb.AttributeArrayValue_DOUBLE_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatFloat(val.DoubleValue, 'f', -1, 64), str)
		return attributeArrayValFromString(replacedValue)
	case pb.AttributeArrayValue_BOOL_VALUE:
		replacedValue := re.ReplaceAllString(strconv.FormatBool(val.BoolValue), str)
		return attributeArrayValFromString(replacedValue)
	default:
		log.Error("Unknown OTEL AttributeArrayValue type %v, replacer code must be updated, replacing unknown type with `?`")
		return &pb.AttributeArrayValue{
			Type:        pb.AttributeArrayValue_STRING_VALUE,
			StringValue: "?",
		}
	}
}

func attributeAnyValFromString(s string) *pb.AttributeAnyValue {
	if rf, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &pb.AttributeAnyValue{
			Type:     pb.AttributeAnyValue_INT_VALUE,
			IntValue: rf,
		}
	} else if rfFloat, err := strconv.ParseFloat(s, 64); err == nil {
		return &pb.AttributeAnyValue{
			Type:        pb.AttributeAnyValue_DOUBLE_VALUE,
			DoubleValue: rfFloat,
		}
		// Restrict bool types to "true" "false" to avoid unexpected type changes
	} else if s == "true" || s == "false" {
		return &pb.AttributeAnyValue{
			Type:      pb.AttributeAnyValue_BOOL_VALUE,
			BoolValue: s == "true",
		}
	}
	return &pb.AttributeAnyValue{
		Type:        pb.AttributeAnyValue_STRING_VALUE,
		StringValue: s,
	}
}

func attributeArrayValFromString(s string) *pb.AttributeArrayValue {
	if rf, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &pb.AttributeArrayValue{
			Type:     pb.AttributeArrayValue_INT_VALUE,
			IntValue: rf,
		}
	} else if rfFloat, err := strconv.ParseFloat(s, 64); err == nil {
		return &pb.AttributeArrayValue{
			Type:        pb.AttributeArrayValue_DOUBLE_VALUE,
			DoubleValue: rfFloat,
		}
		// Restrict bool types to "true" "false" to avoid unexpected type changes
	} else if s == "true" || s == "false" {
		return &pb.AttributeArrayValue{
			Type:      pb.AttributeArrayValue_BOOL_VALUE,
			BoolValue: s == "true",
		}
	}
	return &pb.AttributeArrayValue{
		Type:        pb.AttributeArrayValue_STRING_VALUE,
		StringValue: s,
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
