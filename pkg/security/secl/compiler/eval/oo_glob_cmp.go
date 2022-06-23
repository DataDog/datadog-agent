// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// GlobCmp replaces a pattern matcher with a glob matcher for *file.path fields.
	GlobCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
			if a.ValueType == PatternValueType {
				a.ValueType = GlobValueType
			} else if b.ValueType == PatternValueType {
				b.ValueType = GlobValueType
			}

			return StringEquals(a, b, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.ValueType == PatternValueType {
				a.ValueType = GlobValueType
			} else {
				var values StringValues
				for _, v := range b.Values.GetFieldValues() {
					if v.Type == PatternValueType {
						v.Type = GlobValueType
					}
					values.AppendFieldValue(v)
				}
				b = &StringValuesEvaluator{
					Values: values,
				}
			}

			return StringValuesContains(a, b, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
			if a.ValueType == PatternValueType {
				a.ValueType = GlobValueType
			}

			return StringArrayContains(a, b, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			var values StringValues
			for _, v := range b.Values.GetFieldValues() {
				if v.Type == PatternValueType {
					v.Type = GlobValueType
				}
				values.AppendFieldValue(v)
			}
			b = &StringValuesEvaluator{
				Values: values,
			}

			return StringArrayMatches(a, b, state)
		},
	}
)
