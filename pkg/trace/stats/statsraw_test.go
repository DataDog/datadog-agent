// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/stretchr/testify/assert"
)

func TestGrain(t *testing.T) {
	assert := assert.New(t)

	s := pb.Span{Service: "thing", Name: "other", Resource: "yo"}
	aggr := newAggregationFromSpan(&s, "default")

	b := strings.Builder{}
	aggr.writeKey(&b)
	assert.Equal("env:default,resource:yo,service:thing", b.String())
	assert.Equal(TagSet{Tag{"env", "default"}, Tag{"resource", "yo"}, Tag{"service", "thing"}}, aggr.toTagSet())
}

func TestGrainWithExtraTags(t *testing.T) {
	assert := assert.New(t)

	s := pb.Span{Service: "thing", Name: "other", Resource: "yo", Meta: map[string]string{tagHostname: "host-id", tagVersion: "v0", tagStatusCode: "418"}}
	aggr := newAggregationFromSpan(&s, "default")

	b := strings.Builder{}
	aggr.writeKey(&b)
	assert.Equal("env:default,resource:yo,service:thing,_dd.hostname:host-id,http.status_code:418,version:v0", b.String())
	assert.Equal(TagSet{Tag{"env", "default"}, Tag{"resource", "yo"}, Tag{"service", "thing"}, Tag{"_dd.hostname", "host-id"}, Tag{"http.status_code", "418"}, Tag{"version", "v0"}}, aggr.toTagSet())
}

func BenchmarkHandleSpanRandom(b *testing.B) {
	sb := NewRawBucket(0, 1e9)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		root := traceutil.GetRoot(benchTrace)
		traceutil.ComputeTopLevel(benchTrace)
		wt := NewWeightedTrace(benchTrace, root)
		for _, span := range wt {
			sb.HandleSpan(span, "dev", nil)
		}
	}
}

var benchTrace = pb.Trace{
	&pb.Span{
		Service:  "rails",
		Name:     "web.template",
		Resource: "SELECT user.handle AS user_handle, user.id AS user_id, user.org_id AS user_org_id, user.password AS user_password, user.email AS user_email, user.name AS user_name, user.role AS user_role, user.team AS user_team, user.support AS user_support, user.is_admin AS user_is_admin, user.github_username AS user_github_username, user.github_token AS user_github_token, user.disabled AS user_disabled, user.verified AS user_verified, user.bot AS user_bot, user.created AS user_created, user.modified AS user_modified, user.time_zone AS user_time_zone, user.password_modified AS user_password_modified FROM user WHERE user.id = ? AND user.org_id = ? LIMIT ?",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3fd1ce2fbc1dde9e,
		ParentID: 0x55acf95eafb06955,
		Start:    1548931840954169000,
		Duration: 100000000,
		Error:    403,
		Meta:     map[string]string{"query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s", "in.host": "2a01:e35:2ee1:7160:f66d:4ff:fe71:b690", "out.host": "/dev/null", "in.section": "dogdataprod"},
		Metrics:  map[string]float64{"rowcount": 0.5066325669281033},
		Type:     "redis",
	},
	&pb.Span{
		Service:  "pg-master",
		Name:     "postgres.query",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x57be126d97c3eed2,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931841019932928,
		Duration: 19844796,
		Error:    400,
		Meta:     map[string]string{"user": "leo"},
		Metrics:  map[string]float64{"size": 0.47564235466940796, "rowcount": 0.12453347154800333},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "rails",
		Name:     "sqlalchemy",
		Resource: "GET cache|xxx",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x61973c4d43bd8f04,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840963747104,
		Duration: 3566171,
		Error:    0,
		Meta:     map[string]string{"in.host": "8.8.8.8", "query": "GET beaker:c76db4c3af90410197cf88b0afba4942:session"},
		Metrics:  map[string]float64{"rowcount": 0.276209049435507, "size": 0.18889910131880996},
		Type:     "redis",
	},
	&pb.Span{
		Service:  "pylons",
		Name:     "postgres.query",
		Resource: "events.buckets",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x4541e015c8c62f79,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954371301,
		Duration: 259245,
		Error:    502,
		Meta:     map[string]string{"in.host": "", "out.host": "/dev/null", "query": "\n        -- get_contexts_sub_query[[org:9543 query_id:a135e15e7d batch:1]]\n        WITH sub_contexts as (\n            \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags,\n            org_id\n        FROM vs9543.dim_context c\n        WHERE key = ANY(%(key)s)\n        \n        \n        \n        \n    \n        )\n        \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags\n        FROM sub_contexts c\n        WHERE (c.org_id = %(org_id)s AND c.tags @> %(yes_tags0)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags1)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags2)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags3)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags4)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags5)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags6)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags7)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags8)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags9)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags10)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags11)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags12)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags13)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags14)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags15)s)\n        \n        \n        \n        \n    \n        "},
		Metrics:  map[string]float64{"rowcount": 0.5543063276573277, "size": 0.6196504333337066, "payloads": 0.9689311094466356},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "rails",
		Name:     "postgres.query",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x273710f0da9967a7,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954749862,
		Duration: 161372,
		Error:    0,
		Meta:     map[string]string{"out.section": "-"},
		Metrics:  map[string]float64{"rowcount": 0.2646545763337349},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "web-billing",
		Name:     "web.query",
		Resource: "GET /url/test/fixture/resource/42",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x69ff3ac466831715,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954191909,
		Duration: 9908,
		Error:    0,
		Meta:     map[string]string{"in.section": "replica"},
		Metrics:  map[string]float64{"rowcount": 0.7800384694533715, "payloads": 0.24585482170573683, "loops": 0.3119738365111953, "size": 0.6693070719377765},
		Type:     "sql",
	},
	&pb.Span{
		Service:  "pg-master",
		Name:     "sqlalchemy",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x27dea5ee886c9fbb,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954175872,
		Duration: 2635,
		Error:    400,
		Meta:     map[string]string{"user": "benjamin", "query": "GET beaker:c76db4c3af90410197cf88b0afba4942:session", "out.section": "proxy-XXX"},
		Metrics:  map[string]float64{"payloads": 0.5207323287655542, "loops": 0.4731462684058845, "heap_allocated": 0.5386526456622786, "size": 0.9438291624690298, "rowcount": 0.14536182482282964},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "django",
		Name:     "pylons.controller",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3d34aa36af4e081f,
		ParentID: 0x3fd1ce2fbc1dde9e,
		Start:    1548931840954169013,
		Duration: 370,
		Error:    400,
		Meta:     map[string]string{"in.host": "", "out.host": "/dev/null", "user": "leo", "query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s"},
		Metrics:  map[string]float64{},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "django",
		Name:     "web.query",
		Resource: "events.buckets",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3a51491c82d0b322,
		ParentID: 0x69ff3ac466831715,
		Start:    1548931840954198336,
		Duration: 2474,
		Error:    1,
		Meta:     map[string]string{"out.section": "8080"},
		Metrics:  map[string]float64{"rowcount": 0.9895177718616301},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "pg-master",
		Name:     "pylons.controller",
		Resource: "GET cache|xxx",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x3482d8abba36420f,
		ParentID: 0x69ff3ac466831715,
		Start:    1548931840954192800,
		Duration: 19,
		Error:    1,
		Meta:     map[string]string{"out.host": "datadoghq.com", "in.section": "22"},
		Metrics:  map[string]float64{"rowcount": 0.12186970474265321, "size": 0.4352687905570856},
		Type:     "redis",
	},
	&pb.Span{
		Service:  "web-billing",
		Name:     "web.template",
		Resource: "GET /url/test/fixture/resource/42",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x4c233dc8bfa40958,
		ParentID: 0x69ff3ac466831715,
		Start:    1548931840954191934,
		Duration: 70,
		Error:    400,
		Meta:     map[string]string{"user": "bartek"},
		Metrics:  map[string]float64{"rowcount": 0.3501786556194641},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "pg-master",
		Name:     "postgres.query",
		Resource: "データの犬",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x41546750dfa40643,
		ParentID: 0x61973c4d43bd8f04,
		Start:    1548931840964093798,
		Duration: 2700058,
		Error:    2,
		Meta:     map[string]string{"query": "\n        -- get_contexts_sub_query[[org:9543 query_id:a135e15e7d batch:1]]\n        WITH sub_contexts as (\n            \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags,\n            org_id\n        FROM vs9543.dim_context c\n        WHERE key = ANY(%(key)s)\n        \n        \n        \n        \n    \n        )\n        \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags\n        FROM sub_contexts c\n        WHERE (c.org_id = %(org_id)s AND c.tags @> %(yes_tags0)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags1)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags2)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags3)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags4)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags5)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags6)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags7)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags8)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags9)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags10)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags11)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags12)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags13)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags14)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags15)s)\n        \n        \n        \n        \n    \n        ", "in.host": "2a01:e35:2ee1:7160:f66d:4ff:fe71:b690"},
		Metrics:  map[string]float64{"payloads": 0.737550948148184, "size": 0.5683740489852795, "rowcount": 0.4318616362850698},
		Type:     "lamar",
	},
	&pb.Span{
		Service:  "rails",
		Name:     "web.template",
		Resource: "events.buckets",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x18e45b850b3c1e39,
		ParentID: 0x273710f0da9967a7,
		Start:    1548931840954781284,
		Duration: 126835,
		Error:    0,
		Meta:     map[string]string{"user": "bartek", "query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s", "in.host": "postgres.service.consul", "out.host": "/dev/null"},
		Metrics:  map[string]float64{},
		Type:     "redis",
	},
	&pb.Span{
		Service:  "pylons",
		Name:     "postgres.query",
		Resource: "SELECT user.handle AS user_handle, user.id AS user_id, user.org_id AS user_org_id, user.password AS user_password, user.email AS user_email, user.name AS user_name, user.role AS user_role, user.team AS user_team, user.support AS user_support, user.is_admin AS user_is_admin, user.github_username AS user_github_username, user.github_token AS user_github_token, user.disabled AS user_disabled, user.verified AS user_verified, user.bot AS user_bot, user.created AS user_created, user.modified AS user_modified, user.time_zone AS user_time_zone, user.password_modified AS user_password_modified FROM user WHERE user.id = ? AND user.org_id = ? LIMIT ?",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x4f3f65e058ddbbfc,
		ParentID: 0x273710f0da9967a7,
		Start:    1548931840954752840,
		Duration: 103,
		Error:    403,
		Meta:     map[string]string{"in.section": "22", "out.section": "standby", "user": "bartek", "in.host": "", "out.host": "138.195.130.42"},
		Metrics:  map[string]float64{"payloads": 0.37210733159614523, "rowcount": 0.5264465848403574, "size": 0.025720650418526562},
		Type:     "http",
	},
	&pb.Span{
		Service:  "web-billing",
		Name:     "postgres.query",
		Resource: "GET /url/test/fixture/resource/42",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x7b566c818866ef8b,
		ParentID: 0x273710f0da9967a7,
		Start:    1548931840954749879,
		Duration: 11,
		Error:    400,
		Meta:     map[string]string{"in.host": "postgres.service.consul", "out.host": "datadoghq.com", "user": "bartek", "query": "SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s"},
		Metrics:  map[string]float64{"rowcount": 0.805619107635167},
		Type:     "redis",
	},
	&pb.Span{
		Service:  "pg-master",
		Name:     "web.query",
		Resource: "SELECT user.handle AS user_handle, user.id AS user_id, user.org_id AS user_org_id, user.password AS user_password, user.email AS user_email, user.name AS user_name, user.role AS user_role, user.team AS user_team, user.support AS user_support, user.is_admin AS user_is_admin, user.github_username AS user_github_username, user.github_token AS user_github_token, user.disabled AS user_disabled, user.verified AS user_verified, user.bot AS user_bot, user.created AS user_created, user.modified AS user_modified, user.time_zone AS user_time_zone, user.password_modified AS user_password_modified FROM user WHERE user.id = ? AND user.org_id = ? LIMIT ?",
		TraceID:  0x5df0afd382d351de,
		SpanID:   0x429768d2d13e8697,
		ParentID: 0x4c233dc8bfa40958,
		Start:    1548931840954191942,
		Duration: 37,
		Error:    400,
		Meta:     map[string]string{"out.host": "datadoghq.com", "in.section": "replica", "query": "GET beaker:c76db4c3af90410197cf88b0afba4942:session", "in.host": "8.8.8.8"},
		Metrics:  map[string]float64{"payloads": 0.3779600143407876, "loops": 0.20498295768971775, "size": 0.7947128947983215, "rowcount": 0.7478115781577667},
		Type:     "lamar",
	},
}
