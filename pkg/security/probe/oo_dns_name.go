// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	// DnsNameCmp lower case values before comparing. Important : this operator override doesn't support approvers
	DnsNameCmp = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.GlobCaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.ScalarCaseInsensitive = true
				b.StringCmpOpts.GlobCaseInsensitive = true
			}

			return eval.StringEquals(a, b, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.GlobCaseInsensitive = true
			}

			return eval.StringValuesContains(a, b, opts, state)
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.GlobCaseInsensitive = true
			} else if b.Field != "" {
				b.StringCmpOpts.ScalarCaseInsensitive = true
				b.StringCmpOpts.GlobCaseInsensitive = true
			}

			return eval.StringArrayContains(a, b, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if a.Field != "" {
				a.StringCmpOpts.ScalarCaseInsensitive = true
				a.StringCmpOpts.GlobCaseInsensitive = true
			}

			return eval.StringArrayMatches(a, b, opts, state)
		},
	}
)
