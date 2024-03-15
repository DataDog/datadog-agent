package tracerpayload

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/zstd"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func BuildRandomChunks(numSpans, numChunks int) []*TableTraceChunk {
	st := &StringTable{
		strings:       nil,
		stringIndexes: map[string]uint32{},
	} //shared across tracer chunks
	chunks := make([]*TableTraceChunk, numChunks)
	for c_i := 0; c_i < numChunks; c_i++ {
		spans := make([]*TableSpan, numSpans)
		for i := 0; i < numSpans; i++ {
			pbSpan := RandomSpan()
			meta := map[uint32]uint32{}
			for k, v := range pbSpan.Meta {
				meta[st.Add(k)] = meta[st.Add(v)]
			}
			metrics := map[uint32]float64{}
			for k, v := range pbSpan.Metrics {
				metrics[st.Add(k)] = v
			}
			tableSpan := &TableSpan{
				stringTable: st,
				service:     st.Add(pbSpan.Service),
				name:        st.Add(pbSpan.Name),
				resource:    st.Add(pbSpan.Resource),
				traceID:     pbSpan.TraceID,
				spanID:      pbSpan.SpanID,
				parentID:    pbSpan.ParentID,
				start:       pbSpan.Start,
				duration:    pbSpan.Duration,
				error:       pbSpan.Error,
				meta:        meta,
				metrics:     metrics,
				typ:         st.Add(pbSpan.Type),
			}
			spans[i] = tableSpan
		}
		chunks[c_i] = &TableTraceChunk{
			StringTable:  st,
			Spans:        spans,
			priority:     0,
			origin:       "",
			droppedTrace: false,
		}
	}
	return chunks
}

func ToPB(tableTraceChunks []*TableTraceChunk) *TracerPayloadPb {
	stringTable := tableTraceChunks[0].StringTable.Strings()
	ttcpb := make([]*TableTraceChunkPb, len(tableTraceChunks))
	for ttcI, ttc := range tableTraceChunks {
		spanspb := make([]*TableSpanPb, len(ttc.Spans))
		for i, span := range ttc.Spans {
			spanspb[i] = &TableSpanPb{
				Service:    span.service,
				Name:       span.name,
				Resource:   span.resource,
				TraceID:    span.traceID,
				SpanID:     span.spanID,
				ParentID:   span.parentID,
				Start:      span.start,
				Duration:   span.duration,
				Error:      span.error,
				Meta:       span.meta,
				Metrics:    span.metrics,
				Type:       span.typ,
				MetaStruct: nil,
			}
		}
		ttcpb[ttcI] = &TableTraceChunkPb{
			Priority:     ttc.priority,
			Origin:       0,
			Spans:        spanspb,
			DroppedTrace: ttc.droppedTrace,
		}
	}
	return &TracerPayloadPb{
		ContainerID:     0,
		LanguageName:    0,
		LanguageVersion: 0,
		TracerVersion:   0,
		RuntimeID:       0,
		Chunks:          ttcpb,
		Tags:            nil,
		Env:             0,
		Hostname:        0,
		AppVersion:      0,
		StringTable:     stringTable,
	}
}

func BenchmarkCompressTables(b *testing.B) {
	//b.Run("v5", func(b *testing.B) {
	//	foo := BuildRandomChunks(200, 10)
	//	b.ResetTimer()
	//	for i := 0; i < b.N; i++ {
	//		tpb := ToPB(foo)
	//		bs, err := proto.Marshal(tpb) // where does MarshalVT come from
	//		if err != nil {
	//			panic(err)
	//		}
	//		b.ReportMetric(float64(len(bs)), "bytes/payload")
	//		b.SetBytes(int64(len(bs)))
	//	}
	//})
	b.Run("gzip", func(b *testing.B) {
		chunks := GetTestTraceChunks(10, 200, true)
		tp := &pb.TracerPayload{Chunks: chunks}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bs, err := tp.MarshalVT()
			var zippedBs bytes.Buffer
			gzipw, err := gzip.NewWriterLevel(&zippedBs, gzip.BestSpeed)
			if err != nil {
				panic(err)
			}
			if _, err := gzipw.Write(bs); err != nil {
				panic(err)
			}
			if err := gzipw.Close(); err != nil {
				panic(err)
			}
			b.ReportMetric(float64(zippedBs.Len()), "bytes/payload")
			gzipr, err := gzip.NewReader(&zippedBs)
			if err != nil {
				panic(err)
			}
			_, err = io.ReadAll(gzipr)
			if err != nil {
				panic(err)
			}
			b.ReportMetric(float64(len(bs)), "uncompressed_bytes/payload")
			b.SetBytes(int64(len(bs)))
		}
	})
	b.Run("zstd-bestspeed", func(b *testing.B) {
		chunks := GetTestTraceChunks(10, 200, true)
		tp := &pb.TracerPayload{Chunks: chunks}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bs, err := tp.MarshalVT()
			var zippedBs bytes.Buffer
			zstdw := zstd.NewWriterLevel(&zippedBs, zstd.BestSpeed)
			if err != nil {
				panic(err)
			}
			if _, err := zstdw.Write(bs); err != nil {
				panic(err)
			}
			if err := zstdw.Close(); err != nil {
				panic(err)
			}
			b.ReportMetric(float64(zippedBs.Len()), "bytes/payload")
			zstdr := zstd.NewReader(&zippedBs)
			if err != nil {
				panic(err)
			}
			_, err = io.ReadAll(zstdr)
			if err != nil {
				panic(err)
			}
			b.ReportMetric(float64(len(bs)), "uncompressed_bytes/payload")
			b.SetBytes(int64(len(bs)))
		}
	})
	b.Run("zstd-default", func(b *testing.B) {
		chunks := GetTestTraceChunks(10, 200, true)
		tp := &pb.TracerPayload{Chunks: chunks}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bs, err := tp.MarshalVT()
			var zippedBs bytes.Buffer
			zstdw := zstd.NewWriterLevel(&zippedBs, zstd.DefaultCompression)
			if err != nil {
				panic(err)
			}
			if _, err := zstdw.Write(bs); err != nil {
				panic(err)
			}
			if err := zstdw.Close(); err != nil {
				panic(err)
			}
			b.ReportMetric(float64(zippedBs.Len()), "bytes/payload")
			zstdr := zstd.NewReader(&zippedBs)
			if err != nil {
				panic(err)
			}
			_, err = io.ReadAll(zstdr)
			if err != nil {
				panic(err)
			}
			b.ReportMetric(float64(len(bs)), "uncompressed_bytes/payload")
			b.SetBytes(int64(len(bs)))
		}
	})
}

// GetTestTraceChunks returns a []TraceChunk that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTraceChunks(traceN, size int, realisticIDs bool) []*pb.TraceChunk {
	traces := GetTestTraces(traceN, size, realisticIDs)
	traceChunks := make([]*pb.TraceChunk, 0, len(traces))
	for _, trace := range traces {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Spans: trace,
		})
	}
	return traceChunks
}

// GetTestTraces returns a []Trace that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTraces(traceN, size int, realisticIDs bool) pb.Traces {
	traces := pb.Traces{}

	r := rand.New(rand.NewSource(42))

	for i := 0; i < traceN; i++ {
		// Calculate a trace ID which is predictable (this is why we seed)
		// but still spreads on a wide spectrum so that, among other things,
		// sampling algorithms work in a realistic way.
		traceID := r.Uint64()

		trace := pb.Trace{}
		for j := 0; j < size; j++ {
			span := RandomSpan()
			if realisticIDs {
				// Need to have different span IDs else traces are rejected
				// because they are not correct (indeed, a trace with several
				// spans boasting the same span ID is not valid)
				span.SpanID += uint64(j)
				span.TraceID = traceID
			}
			trace = append(trace, span)
		}
		traces = append(traces, trace)
	}
	return traces
}

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

// RandomSpan generates a wide-variety of spans, useful to test robustness & performance
func RandomSpan() *pb.Span {
	return &pb.Span{
		Duration: RandomSpanDuration(),
		Error:    RandomSpanError(),
		Resource: RandomSpanResource(),
		Service:  RandomSpanService(),
		Name:     RandomSpanName(),
		SpanID:   RandomSpanID(),
		Start:    RandomSpanStart(),
		TraceID:  RandomSpanTraceID(),
		Meta:     RandomSpanMeta(),
		Metrics:  RandomSpanMetrics(),
		ParentID: RandomSpanParentID(),
		Type:     RandomSpanType(),
	}
}

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
