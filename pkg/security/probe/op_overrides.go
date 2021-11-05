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
			var scalar *eval.StringEvaluator
			if a.EvalFnc == nil {
				scalar = a
			} else if b.EvalFnc == nil {
				scalar = b
			} else {
				return nil, errors.New("non scalar overriden is not supported")
			}

			evaluator := eval.StringArrayEvaluator{
				EvalFnc: func(ctx *eval.Context) eval.StringValues {
					values := eval.StringValues{}
					values.AppendEvaluator(scalar)

					return values
				},
			}

			return eval.ArrayStringContains(a, &evaluator, opts, state)
		},
	}
)
