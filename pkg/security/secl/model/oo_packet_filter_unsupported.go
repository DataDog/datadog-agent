// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(unix && pcap && cgo)

// Package model holds model related files
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// PacketFilterMatching is a set of overrides for packet filter fields, it only supports matching a single static value
var PacketFilterMatching = &eval.OpOverrides{
	StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		if a.IsStatic() || b.IsStatic() {
			return &eval.BoolEvaluator{
				Value: false,
			}, nil
		}

		return nil, errorNonStaticPacketFilterField(a, b)
	},
	StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errorNonStaticPacketFilterField(a, b)
	},
	StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errorNonStaticPacketFilterField(a, b)
	},
	StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errorNonStaticPacketFilterField(a, b)
	},
}
