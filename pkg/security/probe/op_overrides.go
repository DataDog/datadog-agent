// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	// OverridePathnames is used to add symlinks to pathnames
	OverridePathnames = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.RuleState) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringEquals(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if !probe.config.SymlinkResolverEnabled {
				return eval.StringEquals(a, b, opts, state)
			}

			var fieldEvaluator *eval.StringEvaluator
			var key unsafe.Pointer
			var scalarValue string

			if a.ValueType == eval.ScalarValueType {
				fieldEvaluator, key, scalarValue = b, unsafe.Pointer(b), a.Value
			} else if b.ValueType == eval.ScalarValueType {
				fieldEvaluator, key, scalarValue = a, unsafe.Pointer(a), b.Value
			} else {
				// non scalar values, no need to use the symlink resolver
				return eval.StringEquals(a, b, opts, state)
			}

			var value eval.StringValues
			value.AppendScalarValue(scalarValue)

			// pre-cache at compile time
			if err := probe.resolvers.SymlinkResolver.InitStringValues(key, fieldEvaluator.Field, &value, state); err != nil {
				return nil, err
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(fieldEvaluator, &evaluator, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.RuleState) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringValuesContains(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if a.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringValuesContains(a, b, opts, state)
			}

			key, values := unsafe.Pointer(b), b.Values

			// pre-cache at compile time
			if err := probe.resolvers.SymlinkResolver.InitStringValues(key, a.Field, &values, state); err != nil {
				return nil, err
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		// ex: process.ancestors.file.path
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.RuleState) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringArrayContains(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if b.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringArrayContains(a, b, opts, state)
			}

			key, scalarValue := unsafe.Pointer(b), a.Value

			var value eval.StringValues
			value.AppendScalarValue(scalarValue)

			// pre-cache at compile time
			if err := probe.resolvers.SymlinkResolver.InitStringValues(key, b.Field, &value, state); err != nil {
				return nil, err
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.RuleState) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringArrayMatches(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if a.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringArrayMatches(a, b, opts, state)
			}

			key, values := unsafe.Pointer(a), b.Values

			// pre-cache at compile time
			if err := probe.resolvers.SymlinkResolver.InitStringValues(key, a.Field, &values, state); err != nil {
				return nil, err
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringArrayMatches(a, &evaluator, opts, state)
		},
	}
)
