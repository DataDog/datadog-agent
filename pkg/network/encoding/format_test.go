// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestFormatRouteIdx(t *testing.T) {

	tests := []struct {
		desc                string
		via                 *network.Via
		routesIn, routesOut map[string]RouteIdx
		idx                 int32
	}{
		{
			desc: "nil via",
			via:  nil,
			idx:  -1,
		},
		{
			desc: "empty via",
			via:  &network.Via{},
			idx:  -1,
		},
		{
			desc:     "empty routes, non-nil via",
			via:      &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:      0,
			routesIn: map[string]RouteIdx{},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:  0,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with non-existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "bar"}},
			idx:  1,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
				"bar": {Idx: 1, Route: model.Route{Subnet: &model.Subnet{Alias: "bar"}}},
			},
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			idx := formatRouteIdx(te.via, te.routesIn)
			require.Equal(t, te.idx, idx)
			require.Len(t, te.routesIn, len(te.routesIn), "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			for k, v := range te.routesOut {
				otherv, ok := te.routesIn[k]
				require.True(t, ok, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
				require.Equal(t, v, otherv, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			}
		})
	}
}

func BenchmarkConnectionReset(b *testing.B) {
	c := new(model.Connection)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Reset()
	}
	runtime.KeepAlive(c)
}

func TestFormatProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol network.ProtocolType
		want     *model.ProtocolStack
	}{
		{
			name:     "unknown protocol",
			protocol: network.ProtocolUnknown,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "unclassified protocol",
			protocol: network.ProtocolUnclassified,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolUnknown,
				},
			},
		},
		{
			name:     "http protocol",
			protocol: network.ProtocolHTTP,
			want: &model.ProtocolStack{
				Stack: []model.ProtocolType{
					model.ProtocolType_protocolHTTP,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, formatProtocol(tt.protocol), "formatProtocol(%v)", tt.protocol)
		})
	}
}
