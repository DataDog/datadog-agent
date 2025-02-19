// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(unix && pcap && cgo)

// Package model holds model related files
package model

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var errUnsupportedPacketFilter = errors.New("packet filter fields are not supported")

// PacketFilterMatching is a set of overrides for packet filter fields, it only supports matching a single static value
var PacketFilterMatching = &eval.OpOverrides{
	StringEquals: func(_ *eval.StringEvaluator, _ *eval.StringEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedPacketFilter
	},
	StringValuesContains: func(_ *eval.StringEvaluator, _ *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedPacketFilter
	},
	StringArrayContains: func(_ *eval.StringEvaluator, _ *eval.StringArrayEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedPacketFilter
	},
	StringArrayMatches: func(_ *eval.StringArrayEvaluator, _ *eval.StringValuesEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		return nil, errUnsupportedPacketFilter
	},
}
