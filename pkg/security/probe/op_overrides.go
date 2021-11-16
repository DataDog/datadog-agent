// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	// OverridePathnames is used to add symlinks to pathnames
	OverridePathnames = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			var value string
			if a.IsScalar() {
				value = a.Value
			} else if b.IsScalar() {
				value = b.Value
			} else {
				return nil, errors.New("non scalar overriden is not supported")
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) eval.StringValues {
					event := (*Event)(ctx.Object)

					values := eval.StringValues{}
					values.AppendValue(value)

					if dest, err := event.resolvers.SymlinkResolver.Resolve(value); err == nil {
						values.AppendValue(dest)
					}

					return values
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) eval.StringValues {
					event := (*Event)(ctx.Object)

					// TODO check not EvalFnc
					values := b.Values

					for _, value := range values.GetScalarValues() {
						values := eval.StringValues{}
						values.AppendValue(value)

						if dest, err := event.resolvers.SymlinkResolver.Resolve(value); err == nil {
							values.AppendValue(dest)
						}
					}

					return values
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		// ex: process.ancestors.file.path
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			var value string
			if a.IsScalar() {
				value = a.Value
			} else {
				return nil, errors.New("non scalar overriden is not supported")
			}

			evaluator := eval.StringValuesEvaluator{
				EvalFnc: func(ctx *eval.Context) eval.StringValues {
					event := (*Event)(ctx.Object)

					values := eval.StringValues{}
					values.AppendValue(value)

					if dest, err := event.resolvers.SymlinkResolver.Resolve(value); err == nil {
						values.AppendValue(dest)
					}

					return values
				},
			}

			return eval.StringValuesContains(a, &evaluator, opts, state)
		},
		/*StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*BoolEvaluator, error) {

		},*/
	}
)
