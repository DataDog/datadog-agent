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
	peerTags := []string{"db.instance", "db.system", "peer.service"}
	peerSvcOnlyHash := uint64(3430395298086625290)
	peerTagsHash := uint64(9894752672193411515)
	for _, tt := range []struct {
		name              string
		in                *pb.Span
		enablePeerTagsAgg bool
		resAgg            Aggregation
		resPeerTags       []string
	}{
		{
			"nil case, peer tag aggregation disabled",
			&pb.Span{},
			false,
			Aggregation{},
			nil,
		},
		{
			"nil case, peer tag aggregation enabled",
			&pb.Span{},
			true,
			Aggregation{},
			nil,
		},
		{
			"peer tag aggregation disabled even though peer.service is present",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "peer.service": "remote-service"},
			},
			false,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client"}},
			nil,
		},
		{
			"peer tags aggregation enabled, but span.kind != (client, producer)",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a"}},
			nil,
		},
		{
			"peer tags aggregation enabled, span.kind == client",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerTagsHash: peerSvcOnlyHash}},
			[]string{"peer.service:remote-service"},
		},
		{
			"peer tags aggregation enabled, span.kind == producer",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "producer", "peer.service": "remote-service"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "producer", PeerTagsHash: peerSvcOnlyHash}},
			[]string{"peer.service:remote-service"},
		},
		{
			"peer tags aggregation enabled and multiple peer tags match",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "field1": "val1", "peer.service": "remote-service", "db.instance": "i-1234", "db.system": "postgres"},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerTagsHash: peerTagsHash}},
			[]string{"db.instance:i-1234", "db.system:postgres", "peer.service:remote-service"},
		},
		{
			"peer tags aggregation enabled but all peer tags are empty",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "field1": "val1", "peer.service": "", "db.instance": "", "db.system": ""},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerTagsHash: 0}},
			nil,
		},
		{
			"peer tags aggregation enabled but some peer tags are empty",
			&pb.Span{
				Service: "a",
				Meta:    map[string]string{"span.kind": "client", "field1": "val1", "peer.service": "remote-service", "db.instance": "", "db.system": ""},
			},
			true,
			Aggregation{BucketsAggregationKey: BucketsAggregationKey{Service: "a", SpanKind: "client", PeerTagsHash: peerSvcOnlyHash}},
			[]string{"peer.service:remote-service"},
		},
	} {
		agg, et := NewAggregationFromSpan(tt.in, "", PayloadAggregationKey{}, tt.enablePeerTagsAgg, peerTags)
		assert.Equal(t, tt.resAgg.Service, agg.Service, tt.name)
		assert.Equal(t, tt.resAgg.SpanKind, agg.SpanKind, tt.name)
		assert.Equal(t, tt.resAgg.PeerTagsHash, agg.PeerTagsHash, tt.name)
		assert.Equal(t, tt.resPeerTags, et, tt.name)
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

func TestIsRootSpan(t *testing.T) {
	for _, tt := range []struct {
		in          *pb.Span
		isTraceRoot pb.TraceRootFlag
	}{
		{
			&pb.Span{},
			pb.TraceRootFlag_TRUE,
		},
		{
			&pb.Span{
				ParentID: 0,
			},
			pb.TraceRootFlag_TRUE,
		},
		{
			&pb.Span{
				ParentID: 123,
			},
			pb.TraceRootFlag_FALSE,
		},
	} {
		agg, _ := NewAggregationFromSpan(tt.in, "", PayloadAggregationKey{}, true, []string{})
		assert.Equal(t, tt.isTraceRoot, agg.IsTraceRoot)
	}
}
