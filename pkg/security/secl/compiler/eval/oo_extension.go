// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import "strings"

var (
	extensionSanitizer = func(kind FieldValueType, pattern string) (string, error) {
		if kind == RegexpValueType || strings.HasPrefix(pattern, ".") {
			return pattern, nil
		}

		return "." + pattern, nil
	}

	// ExtensionCmp normalizes file extension values by stripping a leading dot,
	// allowing rules to use either ".txt" or "txt" to match the same extension.
	ExtensionCmp = &OpOverrides{
		StringEquals: func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.Sanitize = extensionSanitizer
			} else if b.Field != "" {
				b.StringCmpOpts.Sanitize = extensionSanitizer
			}

			return StringEquals(a, b, state)
		},
		StringValuesContains: func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.Sanitize = extensionSanitizer
			}

			return StringValuesContains(a, b, state)
		},
		StringArrayContains: func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.Sanitize = extensionSanitizer
			} else if b.Field != "" {
				b.StringCmpOpts.Sanitize = extensionSanitizer
			}

			return StringArrayContains(a, b, state)
		},
		StringArrayMatches: func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.Sanitize = extensionSanitizer
			}

			return StringArrayMatches(a, b, state)
		},
	}
)
