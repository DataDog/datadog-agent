// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/stretchr/testify/assert"
)

func TestGetStatusCode(t *testing.T) {
	for _, tt := range []struct {
		in  *pb.Span
		out uint32
	}{
		{
			&pb.Span{},
			0,
		},
		{
			&pb.Span{
				Meta: map[string]string{"http.status_code": "200"},
			},
			200,
		},
		{
			&pb.Span{
				Metrics: map[string]float64{"http.status_code": 302},
			},
			302,
		},
		{
			&pb.Span{
				Meta:    map[string]string{"http.status_code": "200"},
				Metrics: map[string]float64{"http.status_code": 302},
			},
			302,
		},
		{
			&pb.Span{
				Meta: map[string]string{"http.status_code": "x"},
			},
			0,
		},
	} {
		if got := getStatusCode(tt.in); got != tt.out {
			t.Fatalf("Expected %d, got %d", tt.out, got)
		}
	}
}

func TestNewAggregation(t *testing.T) {
	for _, tt := range []struct {
		in               *pb.Span
		enablePeerSvcAgg bool
		resAgg           Aggregation
		resPeerTags      []string
	}{
		{
			&pb.Span{},
			false,
			Aggregation{},
			nil,
		},
		{
			&pb.Span{},
			true,
			Aggregation{},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "peer.service": "remote-service"},
			},
			false,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client"}},
			nil,
		},
		// peer.service stats aggregation is enabled, but span.kind != (client, producer).
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a"}},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerService: "remote-service"}},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "producer", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "producer", PeerService: "remote-service"}},
			nil,
		},
		{
			&pb.Span{
				Service:  "service",
				Name:     "operation",
				Resource: "resource",
				Meta: map[string]string{
					"span.kind":        "client",
					"peer.service":     "remote-service",
					"http.status_code": "200",
				},
			},
			true,
			Aggregation{
				BucketsAggregationKey: BucketsAggregationKey{
					Service:     "service",
					Name:        "operation",
					PeerService: "remote-service",
					Resource:    "resource",
					SpanKind:    "client",
					StatusCode:  200,
					Synthetics:  false,
				},
			},
			nil,
		},
	} {
		agg, et := NewAggregationFromSpan(tt.in, "", PayloadAggregationKey{}, tt.enablePeerSvcAgg, nil)
		assert.Equal(t, tt.resAgg, agg)
		assert.Equal(t, tt.resPeerTags, et)
	}
}

func TestNewAggregationPeerTags(t *testing.T) {
	peerTags := []string{"db.instance", "db.system"}
	for _, tt := range []struct {
		in          *pb.Span
		resAgg      Aggregation
		resPeerTags []string
	}{
		{
			&pb.Span{},
			Aggregation{},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "", "field1": "val1", "db.instance": "i-1234", "db.system": "postgres"},
			},
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", PeerTagsHash: 0}},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "server", "field1": "val1", "db.instance": "i-1234", "db.system": "postgres"},
			},
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "server", PeerTagsHash: 0}},
			nil,
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "field1": "val1", "db.instance": "i-1234", "db.system": "postgres"},
			},
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerTagsHash: 17292111254139093926}},
			[]string{"db.instance:i-1234", "db.system:postgres"},
		},
		{
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "producer", "field1": "val1", "db.instance": "i-1234", "db.system": "postgres"},
			},
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "producer", PeerTagsHash: 17292111254139093926}},
			[]string{"db.instance:i-1234", "db.system:postgres"},
		},
	} {
		agg, et := NewAggregationFromSpan(tt.in, "", PayloadAggregationKey{}, true, peerTags)
		assert.Equal(t, tt.resAgg, agg)
		assert.Equal(t, tt.resPeerTags, et)
	}
}

func TestSpanKindIsConsumerOrProducer(t *testing.T) {
	type testCase struct {
		input string
		res   bool
	}
	for _, tc := range []testCase{
		{"client", true},
		{"producer", true},
		{"CLIENT", true},
		{"PRODUCER", true},
		{"cLient", true},
		{"pRoducer", true},
		{"server", false},
		{"consumer", false},
		{"internal", false},
		{"", false},
	} {
		assert.Equal(t, tc.res, clientOrProducer(tc.input))
	}
}
