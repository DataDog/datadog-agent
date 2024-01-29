// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// In this file we define methods and global variables to:
// allow generation of arbitrary/random VALID spans
// pick random attributes for a span

package testutil

import (
	"math/rand"
	"strconv"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// YearNS is the number of nanoseconds in a year
var YearNS = (time.Hour * 24 * 365).Nanoseconds()

var durations = []int64{
	1 * 1e3,   // 1us
	10 * 1e3,  // 10us
	100 * 1e3, // 100us
	1 * 1e6,   // 1ms
	50 * 1e6,  // 50ms
	100 * 1e6, // 100ms
	500 * 1e6, // 500ms
	1 * 1e9,   // 1s
	2 * 1e9,   // 2s
	10 * 1e9,  // 10s
}

var resources = []string{
	"GET cache|xxx",
	"events.buckets",
	"SELECT user.handle AS user_handle, user.id AS user_id, user.org_id AS user_org_id, user.password AS user_password, user.email AS user_email, user.name AS user_name, user.role AS user_role, user.team AS user_team, user.support AS user_support, user.is_admin AS user_is_admin, user.github_username AS user_github_username, user.github_token AS user_github_token, user.disabled AS user_disabled, user.verified AS user_verified, user.bot AS user_bot, user.created AS user_created, user.modified AS user_modified, user.time_zone AS user_time_zone, user.password_modified AS user_password_modified FROM user WHERE user.id = ? AND user.org_id = ? LIMIT ?",
	"データの犬",
	"GET /url/test/fixture/resource/42",
}

var services = []string{
	"mysql-db",
	"postgres-db",
	"gorm",
	"mux",
	"rails",
	"django",
	"web-billing",
	"pg-master",
	"pylons",
}

var names = []string{
	"web.query",
	"sqlalchemy",
	"web.template",
	"pylons.controller",
	"postgres.query",
}

var metas = map[string][]string{
	"sql.query": {
		"GET beaker:c76db4c3af90410197cf88b0afba4942:session",
		"SELECT id\n                 FROM ddsuperuser\n                WHERE id = %(id)s",
		"\n        -- get_contexts_sub_query[[org:9543 query_id:a135e15e7d batch:1]]\n        WITH sub_contexts as (\n            \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags,\n            org_id\n        FROM vs9543.dim_context c\n        WHERE key = ANY(%(key)s)\n        \n        \n        \n        \n    \n        )\n        \n        -- \n        --\n        SELECT key,\n            host_name,\n            device_name,\n            tags\n        FROM sub_contexts c\n        WHERE (c.org_id = %(org_id)s AND c.tags @> %(yes_tags0)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags1)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags2)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags3)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags4)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags5)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags6)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags7)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags8)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags9)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags10)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags11)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags12)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags13)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags14)s)\n        OR (c.org_id = %(org_id)s AND c.tags @> %(yes_tags15)s)\n        \n        \n        \n        \n    \n        ",
	},
	"in.host": {
		"8.8.8.8",
		"172.0.0.42",
		"2a01:e35:2ee1:7160:f66d:4ff:fe71:b690",
		"postgres.service.consul",
		"",
	},
	"http.method": {
		"GET",
		"POST",
		"PUT",
		"DELETE",
		"UPDATED",
	},
	"http.status_code": {
		"400",
		"500",
		"300",
		"200",
		"404",
		"402",
		"401",
		"202",
		"220",
	},
	"out.port": {
		"1233",
		"8124",
		"8125",
		"9999",
		"8888",
		"80",
		"8080",
	},
	"version": {
		"1.2.0",
		"1.0.0",
		"0.2.2-alpha",
		"3.4.4",
		"2.0.0",
		"7.12.0",
	},
	"system.pid": {
		"1322",
		"9021",
		"9911",
		"9000",
		"919",
		"414",
		"788",
	},
	"db.name": {
		"jdbc",
		"users",
		"products",
		"services",
		"accounts",
		"photos",
	},
	"db.user": {
		"root",
		"john",
		"jane",
		"admin",
		"user0",
	},
	"cassandra.row_count": {
		"10",
		"11",
		"12",
		"13",
		"14",
		"50",
	},
	"out.host": {
		"/dev/null",
		"138.195.130.42",
		"raclette.service",
		"datadoghq.com",
	},
	"in.section": {
		"4242",
		"22",
		"dogdataprod",
		"replica",
	},
	"out.section": {
		"-",
		"8080",
		"standby",
		"proxy-XXX",
	},
	"user": {
		"mattp",
		"bartek",
		"benjamin",
		"leo",
	},
}

var spanMetrics = []string{
	"rowcount",
	"size",
	"payloads",
	"loops",
	"heap_allocated",
	"results",
}

var types = []string{
	"web",
	"db",
	"cache",
	"http",
	"sql",
	"redis",
	"cassandra",
	"consul",
	"leveldb",
	"memcached",
}

type sliceRandomizer interface {
	Len() int
	Get(int) interface{}
}

type int64Slice []int64

func (s int64Slice) Len() int              { return len(s) }
func (s int64Slice) Get(i int) interface{} { return s[i] }

type int32Slice []int32

func (s int32Slice) Len() int              { return len(s) }
func (s int32Slice) Get(i int) interface{} { return s[i] }

type stringSlice []string

func (s stringSlice) Len() int              { return len(s) }
func (s stringSlice) Get(i int) interface{} { return s[i] }

func randomChoice(s sliceRandomizer) interface{} {
	if s.Len() == 0 {
		return nil
	}
	return s.Get(rand.Intn(s.Len()))
}

func int64RandomChoice(s []int64) int64 {
	return randomChoice(int64Slice(s)).(int64)
}

func int32RandomChoice(s []int32) int32 {
	return randomChoice(int32Slice(s)).(int32)
}

func stringRandomChoice(s []string) string {
	got := randomChoice(stringSlice(s)).(string)
	runes := []rune(got)
	rand.Shuffle(len(runes), func(x, y int) {
		// Add more randomization by shuffling the characters
		runes[x], runes[y] = runes[y], runes[x]
	})
	return string(runes)
}

// RandomSpanDuration generates a random span duration
func RandomSpanDuration() int64 {
	return int64RandomChoice(durations)
}

// RandomSpanError generates a random span error code
func RandomSpanError() int32 {
	return int32RandomChoice([]int32{0, 1})
}

// RandomSpanResource generates a random span resource string
func RandomSpanResource() string {
	return stringRandomChoice(resources)
}

// RandomSpanService generates a random span service string
func RandomSpanService() string {
	return stringRandomChoice(services)
}

// RandomSpanName generates a random span name string
func RandomSpanName() string {
	return stringRandomChoice(names)
}

// RandomSpanID generates a random span ID
func RandomSpanID() uint64 {
	return uint64(rand.Int63())
}

// RandomSpanStart generates a span start timestamp
func RandomSpanStart() int64 {
	// Make sure spans end in the past
	maxDuration := time.Duration(durations[len(durations)-1])
	offset := time.Duration(rand.Intn(10)) * time.Second
	return time.Now().Add(-1 * maxDuration).Add(-1 * offset).UnixNano()
}

// RandomSpanTraceID generates a random trace ID
func RandomSpanTraceID() uint64 {
	return RandomSpanID()
}

// RandomSpanMeta generates some random span metadata
func RandomSpanMeta() map[string]string {
	res := make(map[string]string)

	// choose some of the keys
	n := rand.Intn(len(metas))
	i := 0
	for k, s := range metas {
		if i > n {
			break
		}
		res[k] = stringRandomChoice(s)
		i++
	}

	return res
}

// RandomSpanMetrics generates some random span metrics
func RandomSpanMetrics() map[string]float64 {
	res := make(map[string]float64)

	// choose some keys
	n := rand.Intn(len(spanMetrics))
	for _, i := range rand.Perm(n) {
		res[spanMetrics[i]] = rand.Float64()
	}

	return res
}

// RandomSpanParentID generates a random span parent ID
func RandomSpanParentID() uint64 {
	return RandomSpanID()
}

// RandomSpanType generates a random span type
func RandomSpanType() string {
	return stringRandomChoice(types)
}

// RandomSpanLink generates a random spanlink
func RandomSpanLink() *pb.SpanLink {
	return &pb.SpanLink{
		TraceID:     RandomSpanTraceID(),
		TraceIDHigh: RandomSpanTraceID(),
		SpanID:      RandomSpanID(),
		Attributes:  RandomStringMap(),
		Tracestate:  strconv.Itoa(rand.Int()),
		Flags:       rand.Uint32() % 2,
	}
}

// RandomSpanLinks generates a random number of random spanlinks
func RandomSpanLinks() []*pb.SpanLink {
	spanLinksCount := rand.Intn(100)
	res := make([]*pb.SpanLink, spanLinksCount)

	for i := range res {
		res[i] = RandomSpanLink()
	}

	return res
}

// RandomSpan generates a wide-variety of spans, useful to test robustness & performance
func RandomSpan() *pb.Span {
	return &pb.Span{
		Duration:  RandomSpanDuration(),
		Error:     RandomSpanError(),
		Resource:  RandomSpanResource(),
		Service:   RandomSpanService(),
		Name:      RandomSpanName(),
		SpanID:    RandomSpanID(),
		Start:     RandomSpanStart(),
		TraceID:   RandomSpanTraceID(),
		Meta:      RandomSpanMeta(),
		Metrics:   RandomSpanMetrics(),
		ParentID:  RandomSpanParentID(),
		Type:      RandomSpanType(),
		SpanLinks: RandomSpanLinks(),
	}
}

// GetTestSpan returns a Span with different fields set
func GetTestSpan() *pb.Span {
	span := &pb.Span{
		TraceID:  42,
		SpanID:   52,
		ParentID: 42,
		Type:     "web",
		Service:  "fennel_IS amazing!",
		Name:     "something &&<@# that should be a metric!",
		Resource: "NOT touched because it is going to be hashed",
		Start:    9223372036854775807,
		Duration: 9223372036854775807,
		Meta:     map[string]string{"http.host": "192.168.0.1"},
		Metrics:  map[string]float64{"http.monitor": 41.99},
		SpanLinks: []*pb.SpanLink{
			{
				TraceID:     42,
				TraceIDHigh: 32,
				SpanID:      52,
				Attributes: map[string]string{
					"a1": "v1",
					"a2": "v2",
				},
				Tracestate: "dd=asdf256,ee=jkl;128",
				Flags:      1 | 1<<31, // 0th bit -> sampling decision, 31st bit -> set/unset
			},
		},
	}
	trace := pb.Trace{span}
	traceutil.ComputeTopLevel(trace)
	return trace[0]
}

// TestSpan returns a fix span with hardcoded info, useful for reproducible tests
func TestSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   42,
		Start:    1472732573337575936,
		TraceID:  424242,
		Meta: map[string]string{
			"user": "leo",
			"pool": "fondue",
		},
		Metrics: map[string]float64{
			"cheese_weight": 100000.0,
		},
		ParentID: 1111,
		Type:     "http",
	}
}
