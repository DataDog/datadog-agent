// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/stretchr/testify/assert"
)

func TestGrain(t *testing.T) {
	assert := assert.New(t)
	s := pb.Span{Service: "thing", Name: "other", Resource: "yo"}
	aggr, _ := NewAggregationFromSpan(&s, "", PayloadAggregationKey{
		Env:         "default",
		Hostname:    "default",
		ContainerID: "cid",
	}, false, nil)
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
			IsTraceRoot: pb.TraceRootFlag_TRUE,
		},
	}, aggr)
}

func TestGrainWithPeerTags(t *testing.T) {
	t.Run("none present", func(t *testing.T) {
		assert := assert.New(t)
		s := pb.Span{
			Service:  "thing",
			Name:     "other",
			Resource: "yo",
			Meta:     map[string]string{"span.kind": "client"},
		}
		aggr, et := NewAggregationFromSpan(&s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		}, true, []string{"aws.s3.bucket", "db.instance", "db.system", "peer.service"})

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
				IsTraceRoot: pb.TraceRootFlag_TRUE,
			},
		}, aggr)
		assert.Nil(et)
	})
	t.Run("partially present", func(t *testing.T) {
		assert := assert.New(t)
		s := pb.Span{
			Service:  "thing",
			Name:     "other",
			Resource: "yo",
			Meta:     map[string]string{"span.kind": "client", "peer.service": "aws-s3", "aws.s3.bucket": "bucket-a"},
		}
		aggr, et := NewAggregationFromSpan(&s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		}, true, []string{"aws.s3.bucket", "db.instance", "db.system", "peer.service"})

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
				IsTraceRoot:  pb.TraceRootFlag_TRUE,
			},
		}, aggr)
		assert.Equal([]string{"aws.s3.bucket:bucket-a", "peer.service:aws-s3"}, et)
	})
	t.Run("all present", func(t *testing.T) {
		assert := assert.New(t)
		s := pb.Span{
			Service:  "thing",
			Name:     "other",
			Resource: "yo",
			Meta:     map[string]string{"span.kind": "client", "peer.service": "aws-dynamodb", "db.instance": "dynamo.test.us1", "db.system": "dynamodb"},
		}
		aggr, et := NewAggregationFromSpan(&s, "", PayloadAggregationKey{
			Env:         "default",
			Hostname:    "default",
			ContainerID: "cid",
		}, true, []string{"db.instance", "db.system", "peer.service"})

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
				IsTraceRoot:  pb.TraceRootFlag_TRUE,
			},
		}, aggr)
		assert.Equal([]string{"db.instance:dynamo.test.us1", "db.system:dynamodb", "peer.service:aws-dynamodb"}, et)
	})
}

func TestGrainWithSynthetics(t *testing.T) {
	assert := assert.New(t)
	s := pb.Span{Service: "thing", Name: "other", Resource: "yo", Meta: map[string]string{tagStatusCode: "418"}}
	aggr, _ := NewAggregationFromSpan(&s, "synthetics-browser", PayloadAggregationKey{
		Hostname:    "host-id",
		Version:     "v0",
		Env:         "default",
		ContainerID: "cid",
	}, false, nil)
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
			IsTraceRoot: pb.TraceRootFlag_TRUE,
		},
	}, aggr)
}

func BenchmarkHandleSpanRandom(b *testing.B) {
	b.Run("no_peer_tags", func(b *testing.B) {
		sb := NewRawBucket(0, 1e9)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, span := range benchSpans {
				sb.HandleSpan(span, 1, true, "", PayloadAggregationKey{"a", "b", "c", "d", "", ""}, false, nil)
			}
		}
	})
	b.Run("peer_tags", func(b *testing.B) {
		sb := NewRawBucket(0, 1e9)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, span := range benchSpans {
				sb.HandleSpan(span, 1, true, "", PayloadAggregationKey{"a", "b", "c", "d", "", ""}, true, defaultPeerTags)
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
