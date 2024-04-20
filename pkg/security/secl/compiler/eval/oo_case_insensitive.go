// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// CaseInsensitiveCmp lower case values before comparing. Important : this operator override doesn't support approvers
	CaseInsensitiveCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.CaseInsensitive = true
			}

			return StringEquals(a, b, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
			}

			return StringValuesContains(a, b, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.CaseInsensitive = true
			}

			return StringArrayContains(a, b, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
			}

			return StringArrayMatches(a, b, state)
		},
	}

	// WindowsPathCmp lower case values before comparing, converts patterns to globs, and normalizes path separators. Important : this operator override doesn't support approvers
	WindowsPathCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
				a.StringCmpOpts.PathSeparatorNormalize = true
			} else if b.Field != "" {
				b.StringCmpOpts.CaseInsensitive = true
				b.StringCmpOpts.PathSeparatorNormalize = true
			}

			return GlobCmp.StringEquals(a, b, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
				a.StringCmpOpts.PathSeparatorNormalize = true
			}

			return GlobCmp.StringValuesContains(a, b, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
				a.StringCmpOpts.PathSeparatorNormalize = true
			} else if b.Field != "" {
				b.StringCmpOpts.CaseInsensitive = true
				b.StringCmpOpts.PathSeparatorNormalize = true
			}

			return GlobCmp.StringArrayContains(a, b, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.CaseInsensitive = true
				a.StringCmpOpts.PathSeparatorNormalize = true
			}

			return GlobCmp.StringArrayMatches(a, b, state)
		},
	}
)
