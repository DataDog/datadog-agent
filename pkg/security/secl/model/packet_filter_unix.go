// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix && pcap && cgo

// Package model holds model related files
package model

import (
	"errors"
	"fmt"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var errNonStaticPacketFilterField = errors.New("packet filter fields only support matching a single static")

func errorNonStaticPacketFilterField(a eval.Evaluator, b eval.Evaluator) error {
	var field string
	if a.IsStatic() {
		field = b.GetField()
	} else if b.IsStatic() {
		field = a.GetField()
	} else {
		return errNonStaticPacketFilterField
	}
	return fmt.Errorf("field `%s` only supports matching a single static value", field)
}

func newPacketFilterEvaluator(field string, value string) (*eval.BoolEvaluator, error) {
	switch field {
	case "packet.filter":
		captureLength := 256 // sizeof(struct raw_packet_t.data)
		filter, err := pcap.NewBPF(layers.LinkTypeEthernet, captureLength, value)
		if err != nil {
			return nil, fmt.Errorf("failed to compile packet filter `%s` on field `%s`: %v", value, field, err)
		}
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				ev.RawPacket.CaptureInfo.Timestamp = ev.ResolveEventTime()
				return filter.Matches(ev.RawPacket.CaptureInfo, ev.RawPacket.Data)
			},
		}, nil
	}
	return nil, fmt.Errorf("field `%s` doesn't support packet filter evaluation", field)
}

// PacketFilterMatching is a set of overrides for packet filter fields, it only supports matching a single static value
var PacketFilterMatching = &eval.OpOverrides{
	StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, _ *eval.State) (*eval.BoolEvaluator, error) {
		if a.IsStatic() {
			return newPacketFilterEvaluator(b.GetField(), a.Value)
		} else if b.IsStatic() {
			return newPacketFilterEvaluator(a.GetField(), b.Value)
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
