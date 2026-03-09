// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// ExtensionCmp normalizes file extension values by stripping a leading dot,
	// allowing rules to use either ".txt" or "txt" to match the same extension.
	ExtensionCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.TrimLeadingDot = true
			} else if b.Field != "" {
				b.StringCmpOpts.TrimLeadingDot = true
			}

			return StringEquals(a, b, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.TrimLeadingDot = true
			}

			return StringValuesContains(a, b, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.TrimLeadingDot = true
			} else if b.Field != "" {
				b.StringCmpOpts.TrimLeadingDot = true
			}

			return StringArrayContains(a, b, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.TrimLeadingDot = true
			}

			return StringArrayMatches(a, b, state)
		},
	}
)
