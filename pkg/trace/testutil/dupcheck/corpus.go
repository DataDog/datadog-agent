// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

// CorpusSpanTypes are the span types of the generated corpus: every type the
// agent branches on in the switch statements of pkg/trace (such as
// obfuscation and OTLP span type mapping), the types used by pkg/trace/testutil,
// the empty type and a type unknown to the agent. The list is maintained by
// hand: nothing checks it against pkg/trace, so update it when the agent
// starts branching on a new span type.
var CorpusSpanTypes = []string{
	"sql", "cassandra", "redis", "valkey", "memcached", "mongodb", "elasticsearch", "opensearch",
	"web", "http", "db", "cache", "custom",
	"consul", "leveldb",
	"", "dupcheck-unknown",
}

// CorpusAgentKeys are attribute keys referenced as constants in pkg/trace
// (obfuscation, normalization, sampling, payload attributes and OTLP
// conversion). The corpus also uses every key of the semantics registry. Like
// CorpusSpanTypes, the list is maintained by hand and nothing checks it
// against pkg/trace.
var CorpusAgentKeys = []string{
	// obfuscation
	"sql.query", "db.statement", "db.query.text", "sql.tables", "db.type", "http.url",
	"redis.raw_command", "valkey.raw_command", "memcached.command", "mongodb.query",
	"elasticsearch.body", "opensearch.body",
	// normalization, sampling and payload attributes
	"_dd.hostname", "_dd.origin", "_dd.p.dm", "_dd.p.tid", "_dd.apm_mode", "_dd.base_service",
	"_dd.git.commit.sha", "_dd.svc_src", "_dd.install.id", "_dd.install.type", "_dd.install.time",
	"_dd.tags.container", "_dd.error_tracking_standalone.error", "_dd.sdk.otlp_export",
	"env", "version", "component", "span.kind", "peer.service", "http.status_code", "http.method",
	// OTLP conversion
	"events", "_dd.span_links", "_dd.span_events.has_exception", "otel.trace_id", "w3c.tracestate",
	"otel.status_code", "otel.status_description", "otel.library.name", "otel.scope.name",
	"error.msg", "error.type", "error.stack", "db.name",
}

// RandomKeyPrefix prefixes the random attribute keys of the corpus, which the
// agent doesn't interpret.
const RandomKeyPrefix = "dupcheck.random."

// SpanSpec is a format-independent description of a span of the corpus.
type SpanSpec struct {
	TraceID, SpanID, ParentID     uint64
	Service, Name, Resource, Type string
	Attributes                    map[string]string
	Events                        []EventSpec
	Links                         []LinkSpec

	// shapes records the value shape of each corpus key set on the span.
	shapes map[string]valueShape
}

// valueShape is how the corpus sets an attribute key on a span.
type valueShape int

const (
	// shapeAbsent leaves the key unset.
	shapeAbsent valueShape = iota
	// shapeUnique sets a value of its own.
	shapeUnique
	// shapeTopLevel sets the value of a top-level field.
	shapeTopLevel
	// shapeOtherAttribute sets the value of another, random, attribute.
	shapeOtherAttribute
	numShapes
)

// EventSpec describes a span event.
type EventSpec struct {
	Name       string
	Attributes map[string]string
}

// LinkSpec describes a span link.
type LinkSpec struct {
	TraceID, SpanID uint64
	Attributes      map[string]string
}

// TraceSpec is a trace of the corpus, its first span being the root.
type TraceSpec struct {
	Spans []SpanSpec
}

// Corpus is a generated, seeded set of traces, rendered in every payload
// format.
type Corpus struct {
	Seed   int64
	Traces []TraceSpec

	rng    *rand.Rand
	nextID uint64
}

const (
	keysPerSpan   = 8
	spansPerTrace = 4
	// shapePasses is the number of passes per span type, one per value shape.
	shapePasses = int(numShapes)
)

// keyShape returns the value shape of the key of index key in pass: over the
// passes of a span type, every key gets every shape.
func keyShape(pass, key int) valueShape {
	return valueShape((pass + key) % int(numShapes))
}

// NewCorpus generates the corpus for seed. For every span type, it makes one
// pass per value shape: in each pass, every key gets a shape deterministically
// (absent, a unique value, the value of a top-level field, or the value of
// another, random, attribute) so that over the passes every key gets every
// shape. The keys that are set are shuffled and spread over spans, the seed
// choosing their placement and the fields they copy.
func NewCorpus(seed int64) *Corpus {
	c := &Corpus{Seed: seed, rng: rand.New(rand.NewSource(seed)), nextID: 1}
	allKeys := corpusKeys()
	for _, typ := range CorpusSpanTypes {
		for pass := range shapePasses {
			shapes := make(map[string]valueShape, len(allKeys))
			var keys []string
			for i, k := range allKeys {
				if shape := keyShape(pass, i); shape != shapeAbsent {
					shapes[k] = shape
					keys = append(keys, k)
				}
			}
			c.rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
			var spans []SpanSpec
			for start := 0; start < len(keys); start += keysPerSpan {
				spans = append(spans, c.span(typ, keys[start:min(start+keysPerSpan, len(keys))], shapes))
			}
			for start := 0; start < len(spans); start += spansPerTrace {
				c.AddTrace(spans[start:min(start+spansPerTrace, len(spans))]...)
			}
		}
	}
	return c
}

// corpusKeys returns the keys of the semantics registry and CorpusAgentKeys, sorted.
func corpusKeys() []string {
	set := make(map[string]bool)
	for _, infos := range semantics.DefaultRegistry().GetAllEquivalences() {
		for _, info := range infos {
			set[info.Name] = true
		}
	}
	for _, k := range CorpusAgentKeys {
		set[k] = true
	}
	return slices.Sorted(maps.Keys(set))
}

// AddTrace adds a trace made of spans to the corpus, assigning its IDs: the
// first span becomes the root and the parent of the others.
func (c *Corpus) AddTrace(spans ...SpanSpec) {
	traceID := c.id()
	t := TraceSpec{Spans: make([]SpanSpec, len(spans))}
	for i, s := range spans {
		s.TraceID = traceID
		if s.SpanID == 0 {
			s.SpanID = c.id()
		}
		if i > 0 {
			s.ParentID = t.Spans[0].SpanID
		}
		for j := range s.Links {
			if s.Links[j].TraceID == 0 {
				s.Links[j].TraceID, s.Links[j].SpanID = traceID, t.Spans[0].SpanID
			}
		}
		t.Spans[i] = s
	}
	c.Traces = append(c.Traces, t)
}

// Batches splits the traces of the corpus into batches of at most size traces.
func (c *Corpus) Batches(size int) [][]TraceSpec {
	var batches [][]TraceSpec
	for start := 0; start < len(c.Traces); start += size {
		batches = append(batches, c.Traces[start:min(start+size, len(c.Traces))])
	}
	return batches
}

func (c *Corpus) id() uint64 {
	c.nextID++
	return c.nextID
}

func (c *Corpus) token() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 12)
	for i := range b {
		b[i] = letters[c.rng.Intn(len(letters))]
	}
	return "v" + string(b)
}

func (c *Corpus) pick(values []string) string {
	return values[c.rng.Intn(len(values))]
}

// span returns a span of type typ setting keys with their shape in shapes.
func (c *Corpus) span(typ string, keys []string, shapes map[string]valueShape) SpanSpec {
	opType := typ
	if opType == "" {
		opType = "none"
	}
	s := SpanSpec{
		Service:    fmt.Sprintf("dupcheck-service-%02d", c.rng.Intn(20)),
		Name:       "dupcheck." + opType + ".operation",
		Resource:   c.resource(typ),
		Type:       typ,
		Attributes: make(map[string]string),
		shapes:     make(map[string]valueShape, len(keys)),
	}
	topLevel := slices.DeleteFunc([]string{s.Service, s.Name, s.Resource, s.Type}, func(v string) bool { return v == "" })
	// partners holds the random attribute sharing the value of each key of
	// shape shapeOtherAttribute.
	partners := make(map[string]string)
	for _, k := range keys {
		s.shapes[k] = shapes[k]
		switch shapes[k] {
		case shapeUnique:
			s.Attributes[k] = c.value(k, typ)
		case shapeTopLevel:
			s.Attributes[k] = c.pick(topLevel)
		case shapeOtherAttribute:
			// The other attribute is a random one, so that the duplicates
			// sent by the SDK don't chain known keys together.
			v := c.value(k, typ)
			partners[k] = RandomKeyPrefix + c.token()
			s.Attributes[k] = v
			s.Attributes[partners[k]] = v
		}
	}
	for range 2 {
		s.Attributes[RandomKeyPrefix+c.token()] = c.token()
	}
	// A unique value may still repeat a top-level field (db.type of a redis
	// span) or the value given to another key, or come from the same template
	// and become equal to it once obfuscated: draw another one. So may the
	// value shared by a key and its partner.
	for _, k := range slices.Sorted(maps.Keys(s.shapes)) {
		switch s.shapes[k] {
		case shapeUnique:
			if s.valueTaken(s.Attributes[k], k) {
				s.Attributes[k] = c.uniqueValue(s, k)
			}
		case shapeOtherAttribute:
			if p := partners[k]; s.valueTaken(s.Attributes[k], k, p) {
				v := c.uniqueValue(s, k, p)
				s.Attributes[k], s.Attributes[p] = v, v
			}
		}
	}
	randomValues := make([]string, 0, len(s.Attributes))
	for _, k := range slices.Sorted(maps.Keys(s.Attributes)) {
		if strings.HasPrefix(k, RandomKeyPrefix) {
			randomValues = append(randomValues, s.Attributes[k])
		}
	}
	// nestedValue returns a value for an event or link attribute: a unique
	// value, the value of a top-level field or the value of a random span
	// attribute.
	nestedValue := func(k string) string {
		switch c.rng.Intn(3) {
		case 1:
			return c.pick(topLevel)
		case 2:
			return c.pick(randomValues)
		}
		return c.value(k, typ)
	}
	if c.rng.Intn(4) == 0 {
		e := EventSpec{Name: "dupcheck.event", Attributes: make(map[string]string)}
		for _, k := range keys[:min(3, len(keys))] {
			e.Attributes[k] = nestedValue(k)
		}
		s.Events = append(s.Events, e)
	}
	if c.rng.Intn(4) == 0 {
		l := LinkSpec{Attributes: map[string]string{"link.name": "dupcheck link " + c.token()}}
		for _, k := range keys[:min(2, len(keys))] {
			l.Attributes[k] = nestedValue(k)
		}
		s.Links = append(s.Links, l)
	}
	return s
}

// valueTaken reports whether v has the same template as a top-level field of
// s or as one of its attributes, the attributes in except excluded.
func (s SpanSpec) valueTaken(v string, except ...string) bool {
	if sameTemplate(v, s.Service) || sameTemplate(v, s.Name) || sameTemplate(v, s.Resource) || sameTemplate(v, s.Type) {
		return true
	}
	for other, ov := range s.Attributes {
		if !slices.Contains(except, other) && sameTemplate(v, ov) {
			return true
		}
	}
	return false
}

// sameTemplate reports whether a and b are equal once their digits are
// removed, such as two queries from the same corpusSQL template: obfuscation
// may make them equal, and the agent would then seem to have copied one into
// the other. Values made only of digits, such as status codes, are compared
// as is.
func sameTemplate(a, b string) bool {
	if a == b {
		return true
	}
	dropDigits := func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}
	ta, tb := strings.Map(dropDigits, a), strings.Map(dropDigits, b)
	return ta != "" && ta == tb
}

// uniqueValue returns a value for key k of span s that has the template of no
// other field of s, the attributes in except excluded: a value shaped for the
// key if one is free, a random token otherwise.
func (c *Corpus) uniqueValue(s SpanSpec, k string, except ...string) string {
	except = append(except, k)
	for range 5 {
		if v := c.value(k, s.Type); !s.valueTaken(v, except...) {
			return v
		}
	}
	for {
		if v := c.token(); !s.valueTaken(v, except...) {
			return v
		}
	}
}

var (
	corpusSQL = []string{
		"SELECT * FROM users WHERE id = %d",
		"UPDATE accounts SET balance = %d WHERE owner = 'alice'",
		"SELECT name, email FROM customers WHERE created_at > '2024-01-%02d'",
		"INSERT INTO orders (id, total) VALUES (%d, 42.5)",
		"DELETE FROM sessions WHERE expires < %d",
	}
	corpusRedis       = []string{"SET session:%d somevalue", "GET user:%d:profile", "HMSET cart:%d item 3 price 10"}
	corpusMemcached   = []string{"set key%d 0 60 5\r\nvalue", "get key%d"}
	corpusMongo       = []string{`{"find": "users", "filter": {"age": %d}}`, `{"insert": "orders", "documents": [{"total": %d}]}`}
	corpusJSONBody    = []string{`{"query": {"match": {"user": "user%d"}}}`, `{"size": %d, "query": {"term": {"status": "active"}}}`}
	corpusURL         = []string{"http://user:password@example.com/api/v1/users/%d?token=secret", "https://example.com/orders/%d/items?card=4111111111111111"}
	corpusCreditCards = []string{"4111 1111 1111 1111", "5105-1051-0510-5100"}
	corpusSpanKinds   = []string{"server", "client", "producer", "consumer", "internal"}
	corpusStatusCodes = []string{"200", "404", "500"}
)

func (c *Corpus) format(templates []string) string {
	return fmt.Sprintf(c.pick(templates), 1+c.rng.Intn(1000))
}

func (c *Corpus) resource(typ string) string {
	switch typ {
	case "sql", "cassandra", "db":
		return c.format(corpusSQL)
	case "redis", "valkey", "cache":
		return c.format(corpusRedis)
	case "memcached":
		return c.format(corpusMemcached)
	case "mongodb":
		return c.format(corpusMongo)
	case "elasticsearch", "opensearch":
		return fmt.Sprintf("GET /index-%d/_search", c.rng.Intn(1000))
	case "web", "http":
		return fmt.Sprintf("GET /api/v1/users/%d", c.rng.Intn(1000))
	}
	return "dupcheck.resource." + c.token()
}

// value returns a new value for key, shaped like what the agent expects under
// that key so that the code paths reading it are exercised.
func (c *Corpus) value(key, typ string) string {
	switch key {
	case "sql.query", "db.statement", "db.query.text":
		return c.format(corpusSQL)
	case "redis.raw_command", "valkey.raw_command":
		return c.format(corpusRedis)
	case "memcached.command":
		return c.format(corpusMemcached)
	case "mongodb.query":
		return c.format(corpusMongo)
	case "elasticsearch.body", "opensearch.body":
		return c.format(corpusJSONBody)
	case "http.url", "url.full":
		return c.format(corpusURL)
	case "span.kind":
		return c.pick(corpusSpanKinds)
	case "http.status_code", "http.response.status_code":
		return c.pick(corpusStatusCodes)
	case "_dd.apm_mode":
		return "edge"
	case "db.type", "db.system", "db.system.name":
		if typ == "redis" || typ == "memcached" {
			return typ
		}
		return "postgresql"
	}
	if c.rng.Intn(20) == 0 {
		return c.pick(corpusCreditCards)
	}
	return c.token()
}

const spanDuration = int64(50 * time.Millisecond)

// TracerPayload renders traces as a v0.4 tracer payload. Chunks are kept by
// the client (priority 2) so that the agent doesn't sample them out.
func TracerPayload(traces []TraceSpec) *pb.TracerPayload {
	now := time.Now().UnixNano()
	tp := &pb.TracerPayload{LanguageName: "go", TracerVersion: "dupcheck", ContainerID: "dupcheck-container"}
	for _, t := range traces {
		chunk := &pb.TraceChunk{Priority: 2}
		for i, s := range t.Spans {
			span := &pb.Span{
				Service:  s.Service,
				Name:     s.Name,
				Resource: s.Resource,
				Type:     s.Type,
				TraceID:  s.TraceID,
				SpanID:   s.SpanID,
				ParentID: s.ParentID,
				Start:    now - spanDuration,
				Duration: spanDuration - int64(i),
				Meta:     maps.Clone(s.Attributes),
				Metrics:  map[string]float64{},
			}
			if i == 0 {
				span.Metrics["_sampling_priority_v1"] = 2
			}
			for _, e := range s.Events {
				attrs := make(map[string]*pb.AttributeAnyValue, len(e.Attributes))
				for k, v := range e.Attributes {
					attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: v}
				}
				span.SpanEvents = append(span.SpanEvents, &pb.SpanEvent{TimeUnixNano: uint64(now), Name: e.Name, Attributes: attrs})
			}
			for _, l := range s.Links {
				span.SpanLinks = append(span.SpanLinks, &pb.SpanLink{TraceID: l.TraceID, SpanID: l.SpanID, Attributes: maps.Clone(l.Attributes)})
			}
			chunk.Spans = append(chunk.Spans, span)
		}
		tp.Chunks = append(tp.Chunks, chunk)
	}
	return tp
}

// TracerPayloadV1 renders traces as a v1 tracer payload.
func TracerPayloadV1(traces []TraceSpec) *idx.InternalTracerPayload {
	now := uint64(time.Now().UnixNano())
	strs := idx.NewStringTable()
	tp := &idx.InternalTracerPayload{Strings: strs, Attributes: make(map[uint32]*idx.AnyValue)}
	tp.SetLanguageName("go")
	tp.SetTracerVersion("dupcheck")
	tp.SetContainerID("dupcheck-container")
	stringAttrs := func(m map[string]string) map[uint32]*idx.AnyValue {
		attrs := make(map[uint32]*idx.AnyValue, len(m))
		for _, k := range slices.Sorted(maps.Keys(m)) {
			attrs[strs.Add(k)] = &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: strs.Add(m[k])}}
		}
		return attrs
	}
	for _, t := range traces {
		var spans []*idx.InternalSpan
		for i, s := range t.Spans {
			span := idx.NewInternalSpan(strs, &idx.Span{
				ServiceRef:  strs.Add(s.Service),
				NameRef:     strs.Add(s.Name),
				ResourceRef: strs.Add(s.Resource),
				TypeRef:     strs.Add(s.Type),
				SpanID:      s.SpanID,
				ParentID:    s.ParentID,
				Start:       now - uint64(spanDuration),
				Duration:    uint64(spanDuration) - uint64(i),
				Attributes:  make(map[uint32]*idx.AnyValue),
			})
			for _, k := range slices.Sorted(maps.Keys(s.Attributes)) {
				span.SetStringAttribute(k, s.Attributes[k])
			}
			ps := span.ToProto()
			for _, e := range s.Events {
				ps.Events = append(ps.Events, &idx.SpanEvent{Time: now, NameRef: strs.Add(e.Name), Attributes: stringAttrs(e.Attributes)})
			}
			for _, l := range s.Links {
				linkTraceID := traceID128(l.TraceID)
				ps.Links = append(ps.Links, &idx.SpanLink{TraceID: linkTraceID[:], SpanID: l.SpanID, Attributes: stringAttrs(l.Attributes)})
			}
			spans = append(spans, span)
		}
		traceID := traceID128(t.Spans[0].TraceID)
		tp.Chunks = append(tp.Chunks, idx.NewInternalTraceChunk(strs, 2, "", make(map[uint32]*idx.AnyValue), spans, false, traceID[:], 0))
	}
	return tp
}

// ResourceSpans renders traces as OTLP resource spans. OTLP spans have no
// service, resource or type: they are carried by the service.name,
// resource.name and span.type span attributes, which take precedence over the
// corpus values of these keys.
func ResourceSpans(traces []TraceSpec) ptrace.ResourceSpans {
	now := time.Now().UnixNano()
	rs := ptrace.NewResourceSpans()
	rs.Resource().Attributes().PutStr("service.name", "dupcheck-otlp-service")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("dupcheck")
	ss.Scope().SetVersion("1.0.0")
	for _, t := range traces {
		for i, s := range t.Spans {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID(traceID128(s.TraceID)))
			span.SetSpanID(spanID64(s.SpanID))
			if s.ParentID != 0 {
				span.SetParentSpanID(spanID64(s.ParentID))
			}
			span.SetName(s.Name)
			span.SetKind(otlpKind(s.Attributes["span.kind"], i == 0))
			span.SetStartTimestamp(pcommon.Timestamp(now - spanDuration))
			span.SetEndTimestamp(pcommon.Timestamp(now - int64(i)))
			attrs := make(map[string]string, len(s.Attributes)+3)
			maps.Copy(attrs, s.Attributes)
			attrs["service.name"] = s.Service
			attrs["resource.name"] = s.Resource
			if s.Type != "" {
				attrs["span.type"] = s.Type
			} else {
				delete(attrs, "span.type")
			}
			for _, k := range slices.Sorted(maps.Keys(attrs)) {
				span.Attributes().PutStr(k, attrs[k])
			}
			for _, e := range s.Events {
				event := span.Events().AppendEmpty()
				event.SetName(e.Name)
				event.SetTimestamp(pcommon.Timestamp(now))
				for _, k := range slices.Sorted(maps.Keys(e.Attributes)) {
					event.Attributes().PutStr(k, e.Attributes[k])
				}
			}
			for _, l := range s.Links {
				link := span.Links().AppendEmpty()
				link.SetTraceID(pcommon.TraceID(traceID128(l.TraceID)))
				link.SetSpanID(spanID64(l.SpanID))
				for _, k := range slices.Sorted(maps.Keys(l.Attributes)) {
					link.Attributes().PutStr(k, l.Attributes[k])
				}
			}
		}
	}
	return rs
}

func otlpKind(kind string, root bool) ptrace.SpanKind {
	switch kind {
	case "server":
		return ptrace.SpanKindServer
	case "client":
		return ptrace.SpanKindClient
	case "producer":
		return ptrace.SpanKindProducer
	case "consumer":
		return ptrace.SpanKindConsumer
	case "internal":
		return ptrace.SpanKindInternal
	}
	if root {
		return ptrace.SpanKindServer
	}
	return ptrace.SpanKindClient
}

// traceID128 returns a 128-bit trace ID whose low 64 bits are id.
func traceID128(id uint64) [16]byte {
	var b [16]byte
	binary.BigEndian.PutUint64(b[8:], id)
	return b
}

func spanID64(id uint64) pcommon.SpanID {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], id)
	return pcommon.SpanID(b)
}
