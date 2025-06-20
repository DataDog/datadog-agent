// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var errUnsupportedOnDemandOp = errors.New("on-demand operator is not supported")

// OnDemandNameOverrides is a set of overrides for on demand name field
var OnDemandNameOverrides = &eval.OpOverrides{
	StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
		return eval.StringEquals(a, b, state)
	},
	StringValuesContains: func(_ *eval.StringEvaluator, _ *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedOnDemandOp
	},
	StringArrayContains: func(_ *eval.StringEvaluator, _ *eval.StringArrayEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedOnDemandOp
	},
	StringArrayMatches: func(_ *eval.StringArrayEvaluator, _ *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedOnDemandOp
	},
}
