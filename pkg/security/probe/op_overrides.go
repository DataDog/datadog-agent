// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
			var fieldEvaluator *eval.StringEvaluator
			var key unsafe.Pointer
			var value string

			if a.IsScalar() {
				fieldEvaluator, key, value = b, unsafe.Pointer(b), a.Value
			} else if b.IsScalar() {
				fieldEvaluator, key, value = a, unsafe.Pointer(a), b.Value
			} else {
				return nil, errors.New("non scalar overriden is not supported")
			}

			// pre-cache at compile time
			probe := opts.UserCtx.(*Probe)
			probe.resolvers.SymlinkResolver.InitStringValues(key, value)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(fieldEvaluator, &evaluator, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if !b.IsScalar() {
				return nil, errors.New("non scalar overriden is not supported")
			}

			// warn regexp
			if len(b.Values.GetRegexValues()) != 0 {
				// TODO
			}

			key, values := unsafe.Pointer(b), b.Values.GetScalarValues()

			// pre-cache at compile time
			probe := opts.UserCtx.(*Probe)
			probe.resolvers.SymlinkResolver.InitStringValues(key, values...)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		// ex: process.ancestors.file.path
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if !a.IsScalar() {
				return nil, errors.New("non scalar overriden is not supported")
			}

			key, value := unsafe.Pointer(b), a.Value

			// pre-cache at compile time
			probe := opts.UserCtx.(*Probe)
			probe.resolvers.SymlinkResolver.InitStringValues(key, value)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			if !b.IsScalar() {
				return nil, errors.New("non scalar overriden is not supported")
			}

			// warn regexp
			if len(b.Values.GetRegexValues()) != 0 {
				// TODO
			}

			key, values := unsafe.Pointer(a), b.Values.GetScalarValues()

			// pre-cache at compile time
			probe := opts.UserCtx.(*Probe)
			probe.resolvers.SymlinkResolver.InitStringValues(key, values...)

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) *eval.StringValues {
					return probe.resolvers.SymlinkResolver.GetStringValues(key)
				},
			}

			return eval.StringArrayMatches(a, &evaluator, opts, state)
		},
	}
)
