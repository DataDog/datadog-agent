// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrain(t *testing.T) {
	assert := assert.New(t)
	s := StatSpan{service: "thing", name: "other", resource: "yo"}
	aggr := NewAggregationFromSpan(&s, "", PayloadAggregationKey{
		Env:         "default",
		Hostname:    "default",
		ContainerID: "cid",
	})
	assert.Equal(Aggregation{
		PayloadAggregationKey: PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		},
		BucketsAggregationKey: BucketsAggregationKey{
			Service:     "thing",
			Name:        "other",
			Resource:    "yo",
			IsTraceRoot: pb.Trilean_TRUE,
		},
	}, aggr)
}

func TestGrainWithPeerTags(t *testing.T) {
	sc := &SpanConcentrator{}
	t.Run("none present", func(t *testing.T) {
		assert := assert.New(t)
		s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
			Service:  "thing",
			Resource: "yo",
			Name:     "other",
			Meta:     map[string]string{"span.kind": "client"},
			Metrics:  map[string]float64{"_dd.measured": 1},
			PeerTags: []string{"aws.s3.bucket", "db.instance", "db.system", "peer.service"},
		})
		aggr := NewAggregationFromSpan(s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		})

		assert.Equal(Aggregation{
			PayloadAggregationKey: PayloadAggregationKey{
				Env:         "default",
				Hostname:    "default",
				ContainerID: "cid",
			},
			BucketsAggregationKey: BucketsAggregationKey{
				Service:     "thing",
				SpanKind:    "client",
				Name:        "other",
				Resource:    "yo",
				IsTraceRoot: pb.Trilean_TRUE,
			},
		}, aggr)
	})
	t.Run("computeBySpanKind config", func(t *testing.T) {
		for _, spanKindEnabled := range []bool{true, false} {
			t.Run(strconv.FormatBool(spanKindEnabled), func(t *testing.T) {
				assert := assert.New(t)
				sci := NewSpanConcentrator(&SpanConcentratorConfig{
					ComputeStatsBySpanKind: spanKindEnabled,
					BucketInterval:         (time.Duration(10) * time.Second).Nanoseconds(),
				}, time.Now().Add(-time.Minute))
				s, _ := sci.NewStatSpanWithConfig(StatSpanConfig{
					Service:  "thing",
					Resource: "yo",
					Name:     "other",
					Meta:     map[string]string{"span.kind": "client", "server.address": "foo"},
					PeerTags: []string{"_dd.base_service", "server.address"},
				})
				if spanKindEnabled {
					assert.Equal([]string{"server.address:foo"}, s.matchingPeerTags)
				} else {
					assert.Nil(s)
				}
			})
		}
	})
	t.Run("service override", func(t *testing.T) {
		for _, spanKind := range []string{"client", "internal"} {
			t.Run(spanKind, func(t *testing.T) {
				assert := assert.New(t)
				s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
					Service:  "thing",
					Resource: "yo",
					Name:     "other",
					Meta:     map[string]string{"span.kind": spanKind, "_dd.base_service": "the-real-base", "server.address": "foo"},
					Metrics:  map[string]float64{"_dd.measured": 1},
					PeerTags: []string{"_dd.base_service", "server.address"},
				})
				if spanKind == "client" {
					assert.Equal([]string{"_dd.base_service:the-real-base", "server.address:foo"}, s.matchingPeerTags)
				} else {
					assert.Equal([]string{"_dd.base_service:the-real-base"}, s.matchingPeerTags)
				}
			})
		}
	})
	t.Run("partially present", func(t *testing.T) {
		assert := assert.New(t)
		meta := map[string]string{"span.kind": "client", "peer.service": "aws-s3", "aws.s3.bucket": "bucket-a"}
		s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
			Service:  "thing",
			Resource: "yo",
			Name:     "other",
			Meta:     meta,
			Metrics:  map[string]float64{"_dd.measured": 1},
			PeerTags: []string{"aws.s3.bucket", "db.instance", "db.system", "peer.service"},
		})

		aggr := NewAggregationFromSpan(s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		})

		assert.Equal(Aggregation{
			PayloadAggregationKey: PayloadAggregationKey{
				Env:         "default",
				Hostname:    "default",
				ContainerID: "cid",
			},
			BucketsAggregationKey: BucketsAggregationKey{
				Service:      "thing",
				SpanKind:     "client",
				Name:         "other",
				Resource:     "yo",
				PeerTagsHash: 13698082192712149795,
				IsTraceRoot:  pb.Trilean_TRUE,
			},
		}, aggr)
	})
	t.Run("peer ip quantization", func(t *testing.T) {
		assert := assert.New(t)
		meta := map[string]string{"span.kind": "client", "server.address": "129.49.218.65"}
		s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
			Service:  "thing",
			Resource: "yo",
			Name:     "other",
			Meta:     meta,
			Metrics:  map[string]float64{"_dd.measured": 1},
			PeerTags: []string{"server.address"},
		})

		aggr := NewAggregationFromSpan(s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		})

		assert.Equal(Aggregation{
			PayloadAggregationKey: PayloadAggregationKey{
				Env:         "default",
				Hostname:    "default",
				ContainerID: "cid",
			},
			BucketsAggregationKey: BucketsAggregationKey{
				Service:      "thing",
				SpanKind:     "client",
				Name:         "other",
				Resource:     "yo",
				PeerTagsHash: 0xad02dc568e7330c5,
				IsTraceRoot:  pb.Trilean_TRUE,
			},
		}, aggr)
		assert.Equal([]string{"server.address:blocked-ip-address"}, s.matchingPeerTags)
	})
	t.Run("all present", func(t *testing.T) {
		assert := assert.New(t)
		meta := map[string]string{"span.kind": "client", "peer.service": "aws-dynamodb", "db.instance": "dynamo.test.us1", "db.system": "dynamodb"}
		s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
			Service:  "thing",
			Resource: "yo",
			Name:     "other",
			Meta:     meta,
			Metrics:  map[string]float64{"_dd.measured": 1},
			PeerTags: []string{"aws.s3.bucket", "db.instance", "db.system", "peer.service"},
		})

		aggr := NewAggregationFromSpan(s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		})

		assert.Equal(Aggregation{
			PayloadAggregationKey: PayloadAggregationKey{
				Env:         "default",
				Hostname:    "default",
				ContainerID: "cid",
			},
			BucketsAggregationKey: BucketsAggregationKey{
				Service:      "thing",
				SpanKind:     "client",
				Name:         "other",
				Resource:     "yo",
				PeerTagsHash: 5537613849774405073,
				IsTraceRoot:  pb.Trilean_TRUE,
			},
		}, aggr)
		assert.Equal([]string{"db.instance:dynamo.test.us1", "db.system:dynamodb", "peer.service:aws-dynamodb"}, s.matchingPeerTags)
	})
}

func TestGrainWithSynthetics(t *testing.T) {
	assert := assert.New(t)
	sc := &SpanConcentrator{}
	meta := map[string]string{traceutil.TagStatusCode: "418"}
	s, _ := sc.NewStatSpanWithConfig(StatSpanConfig{
		Service:  "thing",
		Resource: "yo",
		Name:     "other",
		Meta:     meta,
		Metrics:  map[string]float64{"_dd.measured": 1},
	})

	aggr := NewAggregationFromSpan(s, "synthetics-browser", PayloadAggregationKey{
		Hostname:    "host-id",
		Version:     "v0",
		Env:         "default",
		ContainerID: "cid",
	})

	assert.Equal(Aggregation{
		PayloadAggregationKey: PayloadAggregationKey{
			Hostname:    "host-id",
			Version:     "v0",
			Env:         "default",
			ContainerID: "cid",
		},
		BucketsAggregationKey: BucketsAggregationKey{
			Service:     "thing",
			Resource:    "yo",
			Name:        "other",
			StatusCode:  418,
			Synthetics:  true,
			IsTraceRoot: pb.Trilean_TRUE,
		},
	}, aggr)
}

func newAdditionalMetricTagStatSpan(value string) *StatSpan {
	return &StatSpan{
		service:                      "checkout-service",
		name:                         "checkout.process",
		resource:                     "POST /checkout/process",
		isTopLevel:                   true,
		duration:                     int64(time.Millisecond),
		matchingAdditionalMetricTags: []string{"customer_id:" + value},
	}
}

func TestRawBucketAdditionalMetricTagsCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{AdditionalTags: 2})
	spanA := newAdditionalMetricTagStatSpan("a")
	spanB := newAdditionalMetricTagStatSpan("b")
	spanC := newAdditionalMetricTagStatSpan("c")
	spanD := newAdditionalMetricTagStatSpan("d")
	spanAAgain := newAdditionalMetricTagStatSpan("a")

	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanA, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanB, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{AdditionalTagsCapBlock: true}, sb.HandleSpan(spanC, 1, "", aggKey))
	assert.Equal(t, []string{"customer_id:c"}, spanC.matchingAdditionalMetricTags)
	assert.Equal(t, SpanCollapseResult{AdditionalTagsCapBlock: true}, sb.HandleSpan(spanD, 1, "", aggKey))
	assert.Equal(t, []string{"customer_id:d"}, spanD.matchingAdditionalMetricTags)
	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanAAgain, 1, "", aggKey))
	assert.Equal(t, []string{"customer_id:a"}, spanAAgain.matchingAdditionalMetricTags)

	require.Len(t, sb.data, 3)
	assert.Equal(t, 2, sb.additionalTagsEntries)
	assert.True(t, sb.warnedThisBucket)

	spanAAggr := NewAggregationFromSpan(spanA, "", aggKey)
	spanBAggr := NewAggregationFromSpan(spanB, "", aggKey)
	// maskedAggr uses sentinel tags directly — spanC is no longer mutated by HandleSpan
	maskedAggr := NewAggregationFromSpan(newAdditionalMetricTagStatSpan("tracer_blocked_value"), "", aggKey)

	spanAStats, ok := sb.data[spanAAggr]
	require.True(t, ok)
	assert.Equal(t, 2.0, spanAStats.hits)

	spanBStats, ok := sb.data[spanBAggr]
	require.True(t, ok)
	assert.Equal(t, 1.0, spanBStats.hits)

	maskedStats, ok := sb.data[maskedAggr]
	require.True(t, ok)
	assert.Equal(t, 2.0, maskedStats.hits)
	assert.Equal(t, []string{"customer_id:tracer_blocked_value"}, maskedStats.additionalMetricTags)
}

func TestRawBucketAdditionalMetricTagsCardinalityLimitDefaultNoop(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{})
	spans := []*StatSpan{
		newAdditionalMetricTagStatSpan("a"),
		newAdditionalMetricTagStatSpan("b"),
		newAdditionalMetricTagStatSpan("c"),
	}

	for _, span := range spans {
		assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(span, 1, "", aggKey))
	}

	require.Len(t, sb.data, 3)
	assert.Zero(t, sb.additionalTagsEntries)
	assert.False(t, sb.warnedThisBucket)
	assert.Equal(t, []string{"customer_id:c"}, spans[2].matchingAdditionalMetricTags)
}

func TestSpanConcentratorAdditionalMetricTagsCardinalityLimitResetsPerBucket(t *testing.T) {
	bsize := int64(time.Second)
	sc := NewSpanConcentrator(&SpanConcentratorConfig{
		BucketInterval:                       bsize,
		AdditionalMetricTagsCardinalityLimit: 1,
	}, time.Unix(0, 0))
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}

	firstAdmitted := newAdditionalMetricTagStatSpan("first-admitted")
	firstAdmitted.start = 1
	firstBlocked := newAdditionalMetricTagStatSpan("first-blocked")
	firstBlocked.start = 2
	secondAdmitted := newAdditionalMetricTagStatSpan("second-admitted")
	secondAdmitted.start = bsize + 1
	secondBlocked := newAdditionalMetricTagStatSpan("second-blocked")
	secondBlocked.start = bsize + 2

	sc.addSpan(firstAdmitted, aggKey, infraTags{}, "", 1)
	sc.addSpan(firstBlocked, aggKey, infraTags{}, "", 1)
	sc.addSpan(secondAdmitted, aggKey, infraTags{}, "", 1)
	sc.addSpan(secondBlocked, aggKey, infraTags{}, "", 1)

	assert.Equal(t, []string{"customer_id:first-admitted"}, firstAdmitted.matchingAdditionalMetricTags)
	assert.Equal(t, []string{"customer_id:first-blocked"}, firstBlocked.matchingAdditionalMetricTags)
	assert.Equal(t, []string{"customer_id:second-admitted"}, secondAdmitted.matchingAdditionalMetricTags)
	assert.Equal(t, []string{"customer_id:second-blocked"}, secondBlocked.matchingAdditionalMetricTags)

	require.Len(t, sc.buckets, 2)
	firstBucket, ok := sc.buckets[0]
	require.True(t, ok)
	secondBucket, ok := sc.buckets[bsize]
	require.True(t, ok)

	assert.Equal(t, 1, firstBucket.additionalTagsEntries)
	assert.True(t, firstBucket.warnedThisBucket)
	assert.Equal(t, 1, secondBucket.additionalTagsEntries)
	assert.True(t, secondBucket.warnedThisBucket)
	assert.Equal(t, BlockCounts{CapBlocks: 2}, sc.DrainBlockCounts())
	assert.Equal(t, BlockCounts{}, sc.DrainBlockCounts())
}

func TestRawBucketResourceCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{Resource: 2})

	mkSpan := func(resource string) *StatSpan {
		return &StatSpan{service: "svc", name: "op", resource: resource, isTopLevel: true, duration: 1}
	}

	spanA := mkSpan("GET /a")
	spanB := mkSpan("GET /b")
	spanC := mkSpan("GET /c")

	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanA, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanB, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{ResourceCollapsed: true}, sb.HandleSpan(spanC, 1, "", aggKey))

	// span not mutated
	assert.Equal(t, "GET /c", spanC.resource)

	// two distinct resource entries + one sentinel entry
	require.Len(t, sb.data, 3)

	sentinel := sb.getAdditionalMetricTagValueBlockSentinel()
	sentinelSpan := mkSpan(sentinel)
	sentinelAggr := NewAggregationFromSpan(sentinelSpan, "", aggKey)
	sentinelStats, ok := sb.data[sentinelAggr]
	require.True(t, ok)
	assert.Equal(t, 1.0, sentinelStats.hits)
}

func TestRawBucketHTTPEndpointCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{HTTPEndpoint: 1})

	mkSpan := func(endpoint string) *StatSpan {
		return &StatSpan{service: "svc", name: "op", resource: "r", httpEndpoint: endpoint, isTopLevel: true, duration: 1}
	}

	spanA := mkSpan("/users")
	spanB := mkSpan("/orders")

	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanA, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{HTTPEndpointCollapsed: true}, sb.HandleSpan(spanB, 1, "", aggKey))

	assert.Equal(t, "/orders", spanB.httpEndpoint)
	require.Len(t, sb.data, 2)
}

func TestRawBucketPeerTagsCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{PeerTags: 1})

	mkSpan := func(peerTags []string) *StatSpan {
		return &StatSpan{service: "svc", name: "op", resource: "r", matchingPeerTags: peerTags, isTopLevel: true, duration: 1}
	}

	spanA := mkSpan([]string{"db.hostname:hostA"})
	spanB := mkSpan([]string{"db.hostname:hostB"})

	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanA, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{PeerTagsCollapsed: true}, sb.HandleSpan(spanB, 1, "", aggKey))

	// span not mutated
	assert.Equal(t, []string{"db.hostname:hostB"}, spanB.matchingPeerTags)
	require.Len(t, sb.data, 2)
}

func TestRawBucketOriginCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{Origin: 1})

	s := &StatSpan{service: "svc", name: "op", resource: "r", isTopLevel: true, duration: 1}

	// "synthetics-user" prefix makes Synthetics=true in the aggregation key, giving two distinct keys.
	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(s, 1, "synthetics-user", aggKey))
	assert.Equal(t, SpanCollapseResult{OriginCollapsed: true}, sb.HandleSpan(s, 1, "other-origin", aggKey))

	// collapsed origin must produce Synthetics=false
	require.Len(t, sb.data, 2)
	for aggr := range sb.data {
		if aggr.BucketsAggregationKey.Synthetics == false && aggr.BucketsAggregationKey.Service == "svc" {
			return // found the collapsed (non-synthetics) entry
		}
	}
	t.Error("expected a collapsed non-synthetics aggregation entry")
}

func TestRawBucketWholeKeyCardinalityLimit(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{WholeKey: 2})

	mkSpan := func(resource string) *StatSpan {
		return &StatSpan{service: "svc", name: "op", resource: resource, isTopLevel: true, duration: 1}
	}

	spanA := mkSpan("GET /a")
	spanB := mkSpan("GET /b")
	spanC := mkSpan("GET /c")

	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanA, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{}, sb.HandleSpan(spanB, 1, "", aggKey))
	assert.Equal(t, SpanCollapseResult{WholeKeyCollapsed: true}, sb.HandleSpan(spanC, 1, "", aggKey))

	// span not mutated
	assert.Equal(t, "GET /c", spanC.resource)

	// two distinct entries + one whole-key sentinel entry
	require.Len(t, sb.data, 3)
	assert.True(t, sb.warnedThisBucket)

	sentinel := sb.getAdditionalMetricTagValueBlockSentinel()
	for aggr, gs := range sb.data {
		if aggr.BucketsAggregationKey.Resource == sentinel {
			assert.Equal(t, 1.0, gs.hits)
			assert.Equal(t, sentinel, aggr.BucketsAggregationKey.Service)
			assert.Equal(t, sentinel, aggr.BucketsAggregationKey.Name)
			return
		}
	}
	t.Error("expected a whole-key sentinel aggregation entry")
}

func TestRawBucketCardinalityLimitsDefaultNoop(t *testing.T) {
	aggKey := PayloadAggregationKey{Env: "prod", Hostname: "host"}
	sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{})

	for i := 0; i < 20; i++ {
		s := &StatSpan{
			service:          "svc",
			name:             "op",
			resource:         fmt.Sprintf("GET /%d", i),
			httpEndpoint:     fmt.Sprintf("/ep%d", i),
			matchingPeerTags: []string{fmt.Sprintf("host:%d", i)},
			isTopLevel:       true,
			duration:         1,
		}
		result := sb.HandleSpan(s, 1, fmt.Sprintf("origin-%d", i), aggKey)
		assert.Equal(t, SpanCollapseResult{}, result, "limits should be no-op when all zero")
	}
	assert.Len(t, sb.data, 20)
}

func BenchmarkHandleSpanRandom(b *testing.B) {
	sc := NewSpanConcentrator(&SpanConcentratorConfig{}, time.Now())
	b.Run("no_peer_tags", func(b *testing.B) {
		sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{})
		var benchStatSpans []*StatSpan
		for _, s := range benchSpans {
			statSpan, ok := sc.NewStatSpanFromPB(s, nil, nil)
			assert.True(b, ok, "Statically defined benchmark spans should require stats")
			benchStatSpans = append(benchStatSpans, statSpan)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, span := range benchStatSpans {
				sb.HandleSpan(span, 1, "", PayloadAggregationKey{Env: "a", Hostname: "b", Version: "c", ContainerID: "d"})
			}
		}
	})
	// This is copied from comp/trace/config/peer_tags_test.go
	// The actual values are not necessarily relevant for the benchmark
	// but we should update them periodically.
	peerTags := []string{
		"_dd.base_service",
		"amqp.destination",
		"amqp.exchange",
		"amqp.queue",
		"aws.queue.name",
		"aws.s3.bucket",
		"bucketname",
		"cassandra.keyspace",
		"db.cassandra.contact.points",
		"db.couchbase.seed.nodes",
		"db.hostname",
		"db.instance",
		"db.name",
		"db.namespace",
		"db.system",
		"grpc.host",
		"hostname",
		"http.host",
		"http.server_name",
		"messaging.destination",
		"messaging.destination.name",
		"messaging.kafka.bootstrap.servers",
		"messaging.rabbitmq.exchange",
		"messaging.system",
		"mongodb.db",
		"msmq.queue.path",
		"net.peer.name",
		"network.destination.name",
		"peer.hostname",
		"peer.service",
		"queuename",
		"rpc.service",
		"rpc.system",
		"server.address",
		"streamname",
		"tablename",
		"topicname",
	}
	b.Run("peer_tags", func(b *testing.B) {
		sb := NewRawBucket(0, 1e9, BucketCardinalityLimits{})
		var benchStatSpans []*StatSpan
		for _, s := range benchSpans {
			statSpan, ok := sc.NewStatSpanFromPB(s, peerTags, nil)
			assert.True(b, ok, "Statically defined benchmark spans should require stats")
			benchStatSpans = append(benchStatSpans, statSpan)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, span := range benchStatSpans {
				sb.HandleSpan(span, 1, "", PayloadAggregationKey{Env: "a", Hostname: "b", Version: "c", ContainerID: "d"})
			}
		}
	})
}

var benchSpans = []*pb.Span{
	{
		Service:  "rails",
		Name:     "web.template",
		Resource: "SELECT user.handle AS user_handle, user.id AS user_id, user.org_id AS user_org_id, user.password AS user_password, user.email AS user_email, user.name AS user_name, user.role AS user_role, user.team AS user_team, user.support AS user_support, user.is_admin AS user_is_admin, user.github_username AS user_github_username, user.github_token AS user_github_token, user.disabled AS user_disabled, user.verified AS user_verified, user.bot AS user_bot, user.created AS user_created, user.modified AS user_modified, user.time_zone AS user_time_zone, user.password_modified AS user_password_modified FROM user WHERE user.id = ? AND user.org_id = ? LIMIT ?",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3fd1ce2fbc1dde9e,
		ParentID: 0x55acf95eafb06955,
		Start:    1548931840954169000,
		Duration: 100000000,
		Error:    403,
		Meta:     map[string]string{"query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s", "db.hostname": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"rowcount": 0.5066325669281033},
		Type:     "",
	},
	{
		Service:  "pg-master",
		Name:     "postgres.query",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x57be126d97c3eed2,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931841019932928,
		Duration: 19844796,
		Error:    400,
		Meta:     map[string]string{"user": "leo", "db.hostname:": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"size": 0.47564235466940796, "rowcount": 0.12453347154800333},
		Type:     "lamar",
	},
	{
		Service:  "rails",
		Name:     "sqlalchemy",
		Resource: "GET cache|xxx",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x61973c4d43bd8f04,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840963747104,
		Duration: 3566171,
		Error:    0,
		Meta:     map[string]string{"in.host": "8.8.8.8", "query": "GET beaker:c76db4c3af90410197cf88b0afba4942:session", "db.hostname:": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"rowcount": 0.276209049435507, "size": 0.18889910131880996},
		Type:     "redis",
	},
	{
		Service:  "pylons",
		Name:     "postgres.query",
		Resource: "events.buckets",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x4541e015c8c62f79,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954371301,
		Duration: 259245,
		Error:    502,
		Meta:     map[string]string{"db.hostname:": "db.host.us1.prod", "db.name": "postgres", "query": "\n        -- get_contexts_sub_query[[org:9543 query_id:a135e15e7d batch:1]]\n        WITH sub_contexts as (\n            \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags,\n            org_id\n        FROM vs9543.dim_context c\n        WHERE key = ANY(%(key)s)\n        \n        \n        \n        \n    \n        )\n        \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags\n        FROM sub_contexts c\n        WHERE (c.org_id = %(org_id)s AND c.tags @> %(yes_tags0)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags1)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags2)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags3)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags4)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags5)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags6)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags7)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags8)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags9)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags10)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags11)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags12)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags13)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags14)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags15)s)\n        \n        \n        \n        \n    \n        "},
		Metrics:  map[string]float64{"rowcount": 0.5543063276573277, "size": 0.6196504333337066, "payloads": 0.9689311094466356},
		Type:     "lamar",
	},
	{
		Service:  "rails",
		Name:     "postgres.query",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x273710f0da9967a7,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954749862,
		Duration: 161372,
		Error:    0,
		Meta:     map[string]string{"out.section": "-", "db.hostname:": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"rowcount": 0.2646545763337349},
		Type:     "lamar",
	},
	{
		Service:  "web-billing",
		Name:     "web.query",
		Resource: "GET /url/test/fixture/resource/42",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x69ff3ac466831715,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954191909,
		Duration: 9908,
		Error:    0,
		Meta:     map[string]string{"peer.service": "foo", "net.peer.name": "foo.us1", "network.destination.name": "foo.us1.12345"},
		Metrics:  map[string]float64{"rowcount": 0.7800384694533715, "payloads": 0.24585482170573683, "loops": 0.3119738365111953, "size": 0.6693070719377765},
		Type:     "sql",
	},
	{
		Service:  "pg-master",
		Name:     "sqlalchemy",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x27dea5ee886c9fbb,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954175872,
		Duration: 2635,
		Error:    400,
		Meta:     map[string]string{"user": "benjamin", "query": "GET beaker:c76db4c3af90410197cf88b0afba4942:session", "db.hostname:": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"payloads": 0.5207323287655542, "loops": 0.4731462684058845, "heap_allocated": 0.5386526456622786, "size": 0.9438291624690298, "rowcount": 0.14536182482282964},
		Type:     "lamar",
	},
	{
		Service:  "django",
		Name:     "pylons.controller",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3d34aa36af4e081f,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954169013,
		Duration: 370,
		Error:    400,
		Meta:     map[string]string{"db.hostname:": "db.host.us1.prod", "db.name": "postgres", "user": "leo", "query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s"},
		Metrics:  map[string]float64{},
		Type:     "lamar",
	},
	{
		Service:  "django",
		Name:     "grpc.client.request",
		Resource: "events.buckets",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3a51491c82d0b322,
		ParentID: 0x69ff3ac466831715,
		Start:    1548931840954198336,
		Duration: 2474,
		Error:    1,
		Meta:     map[string]string{"rpc.service": "buckets", "out.host": "baz", "net.peer.name": "baz.us1", "network.destination.name": "baz.us1.12345"},
		Metrics:  map[string]float64{"rowcount": 0.9895177718616301},
		Type:     "lamar",
	},
	{
		Service:  "django",
		Name:     "postgres.query",
		Resource: "SELECT id FROM table;",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3fd1ce2fbc1dde9e,
		ParentID: 0x3a51491c82d0b322,
		Start:    1548931840954169000,
		Duration: 100000000,
		Error:    403,
		Meta:     map[string]string{"query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s", "db.hostname": "db.host.us1.prod", "db.name": "postgres"},
		Metrics:  map[string]float64{"rowcount": 0.5066325669281033},
		Type:     "db",
	},
}

const roundMask int64 = 1 << 10

func oldNSTimestampToFloat(ns int64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}

func TestNSTimestampToFloat(t *testing.T) {
	ns := []int64{
		int64(1066789584153112 - 1066789583298779), // kernel boot time values
		int64(0),
		int64(1),
		int64(1066789584153112),
		int64(time.Hour * 24 * 3650), // 10 year
		int64(time.Now().UnixNano()),
		int64(0x000000000000ffff),
		int64(1023),
		int64(1024),
		int64(1025),
		//^int64(0), this can't be used here because float64 have only 52 bits of mantissa
		// and filter(float(int64)) will difference due to roundup than float(filter(int64))
		int64(0x001fffffffffffff),
		^int64(0x001fffffffffffff), // ~584 years
	}

	for _, n := range ns {
		assert.Equal(t, oldNSTimestampToFloat(n), nsTimestampToFloat(n), "uint64 10 bits mantissa truncation failed "+fmt.Sprintf("%d 0x%x", n, n))
	}
}
