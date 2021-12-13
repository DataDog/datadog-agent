// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	// OverridePathnames is used to add symlinks to pathnames
	OverridePathnames = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringEquals(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if !probe.config.SymlinkResolverEnabled {
				return eval.StringEquals(a, b, opts, state)
			}

			var fieldEvaluator *eval.StringEvaluator
			var key unsafe.Pointer
			var value string

			if a.IsScalar() {
				fieldEvaluator, key, value = b, unsafe.Pointer(b), a.Value
			} else if b.IsScalar() {
				fieldEvaluator, key, value = a, unsafe.Pointer(a), b.Value
			} else {
				return nil, errors.New("non scalar overridden is not supported")
			}

			if fieldEvaluator.Field == "" {
				return eval.StringEquals(a, b, opts, state)
			}

			// pre-cache at compile time
			probe.resolvers.SymlinkResolver.InitStringValues(key, fieldEvaluator.Field, value)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(fieldEvaluator, &evaluator, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringValuesContains(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if a.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringValuesContains(a, b, opts, state)
			}

			if !b.IsScalar() {
				return nil, errors.New("non scalar overridden is not supported")
			}

			key, values := unsafe.Pointer(b), b.Values.GetScalarValues()

			// pre-cache at compile time
			probe.resolvers.SymlinkResolver.InitStringValues(key, a.Field, values...)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		// ex: process.ancestors.file.path
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringArrayContains(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if b.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringArrayContains(a, b, opts, state)
			}

			if !a.IsScalar() {
				return nil, errors.New("non scalar overridden is not supported")
			}

			key, value := unsafe.Pointer(b), a.Value

			// pre-cache at compile time
			probe.resolvers.SymlinkResolver.InitStringValues(key, b.Field, value)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if opts.UserCtx == nil {
				return eval.StringArrayMatches(a, b, opts, state)
			}
			probe := opts.UserCtx.(*Probe)

			if a.Field == "" || !probe.config.SymlinkResolverEnabled {
				return eval.StringArrayMatches(a, b, opts, state)
			}

			if !b.IsScalar() {
				return nil, errors.New("non scalar overridden is not supported")
			}

			key, values := unsafe.Pointer(a), b.Values.GetScalarValues()

			// pre-cache at compile time
			probe.resolvers.SymlinkResolver.InitStringValues(key, a.Field, values...)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringArrayMatches(a, &evaluator, opts, state)
		},
	}
)
