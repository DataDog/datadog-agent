// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// DNSNameCmp lower case values before comparing. Important : this operator override doesn't support approvers
	DNSNameCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, replCtx ReplacementContext, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.PatternCaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.ScalarCaseInsensitive = true
				b.StringCmpOpts.PatternCaseInsensitive = true
			}

			return StringEquals(a, b, replCtx, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, replCtx ReplacementContext, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.PatternCaseInsensitive = true
			}

			return StringValuesContains(a, b, replCtx, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, replCtx ReplacementContext, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.PatternCaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.ScalarCaseInsensitive = true
				b.StringCmpOpts.PatternCaseInsensitive = true
			}

			return StringArrayContains(a, b, replCtx, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, replCtx ReplacementContext, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.PatternCaseInsensitive = true
			}

			return StringArrayMatches(a, b, replCtx, state)
		},
	}
)
