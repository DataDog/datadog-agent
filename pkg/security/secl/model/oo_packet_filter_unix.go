// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func newPacketFilterEvaluator(field string, value string, state *eval.State) (*eval.BoolEvaluator, error) {
	switch field {
	case "packet.filter":
		captureLength := 256 // sizeof(struct raw_packet_t.data)
		filter, err := libpcap.NewBPF(libpcap.LinkTypeEthernet, captureLength, value)
		if err != nil {
			return nil, fmt.Errorf("failed to compile packet filter `%s` on field `%s`: %v", value, field, err)
		}

		// needed to track filter values and to apply tc filters
		if err := state.UpdateFieldValues(field, eval.FieldValue{Value: value, Type: eval.ScalarValueType}); err != nil {
			return nil, err
		}

		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				ci := libpcap.CaptureInfo{
					Length:        int(ev.RawPacket.NetworkContext.Size),
					CaptureLength: len(ev.RawPacket.Data),
				}
				return filter.Matches(ci, ev.RawPacket.Data)
			},
		}, nil
	}
	return nil, fmt.Errorf("field `%s` doesn't support packet filter evaluation", field)
}

// PacketFilterMatching is a set of overrides for packet filter fields, it only supports matching a single static value
var PacketFilterMatching = &eval.OpOverrides{
	StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
		if a.IsStatic() {
			return newPacketFilterEvaluator(b.GetField(), a.Value, state)
		} else if b.IsStatic() {
			return newPacketFilterEvaluator(a.GetField(), b.Value, state)
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
