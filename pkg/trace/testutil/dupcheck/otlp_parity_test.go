// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	gzipcomp "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil/dupcheck"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// otlpParityConfigs are the configurations of TestOTLPPipelineParity: the
// dupcheck matrix plus the features that change how OTLP traces are converted
// or processed.
func otlpParityConfigs() []dupcheckConfig {
	feature := func(name string) dupcheckConfig {
		return dupcheckConfig{name: name, configure: func(c *config.AgentConfig) { c.Features[name] = struct{}{} }}
	}
	return append(slices.Clone(dupcheckConfigs),
		feature("enable_otlp_compute_top_level_by_span_kind"),
		feature("disable_operation_and_resource_name_logic_v2"),
		feature("disable_otlp_container_tags_v2"),
		dupcheckConfig{name: "span-name-as-resource-name", configure: func(c *config.AgentConfig) { c.OTLPReceiver.SpanNameAsResourceName = true }},
		dupcheckConfig{name: "probabilistic-sampler", configure: func(c *config.AgentConfig) {
			c.ProbabilisticSamplerEnabled = true
			c.ProbabilisticSamplerSamplingPercentage = 50
		}},
		dupcheckConfig{name: "otlp-probabilistic-sampling", configure: func(c *config.AgentConfig) { c.OTLPReceiver.ProbabilisticSampling = 50 }},
	)
}

// TestOTLPPipelineParity checks that OTLP traces processed by the V1 pipeline
// (the default: OTLP receiver → Agent.InV1 → ProcessV1 → TraceWriterV1) give
// the same outbound spans, the same APM stats and the same receiver telemetry
// as by the legacy pipeline (disable-convert-traces: OTLP receiver → Agent.In
// → Process → TraceWriter). The two agents are identical apart from that
// feature. otlpPipelineParityExceptions lists the expected differences.
//
// The inputs are run in two groups, each by its own pair of agents since the
// samplers keep state across payloads:
//   - consistent: the dupcheck corpus where the spans of a resource share the
//     env and version of their resource, as with OTel SDKs, and the
//     hand-written inputs. Everything is compared, APM stats included.
//   - divergent: the raw dupcheck corpus, where every span has its own random
//     env and version. The payload env and app version, which both pipelines
//     promote from a span but not from the same one, differ, and so do the
//     sampler rates and the APM stats, which are keyed by them.
func TestOTLPPipelineParity(t *testing.T) {
	corpus := newDupcheckInputs(t, dupcheck.NewCorpus(*dupcheckSeed)).otlp
	consistent := make([]ptrace.ResourceSpans, 0, len(corpus)+2)
	for i, rs := range corpus {
		consistent = append(consistent, withResourceEnvAndVersion(rs, fmt.Sprintf("parity-env-%d", i%3), fmt.Sprintf("1.0.%d", i%2)))
	}
	consistent = append(consistent, otlpParityResourceSpans()...)
	now := time.Now()
	for _, c := range otlpParityConfigs() {
		t.Run(c.name, func(t *testing.T) {
			for _, group := range []struct {
				name      string
				inputs    []ptrace.ResourceSpans
				divergent bool
			}{
				{"consistent", consistent, false},
				{"divergent", corpus, true},
			} {
				t.Run(group.name, func(t *testing.T) {
					legacy := newOTLPParityAgent(t, c, true, now)
					v1 := newOTLPParityAgent(t, c, false, now)
					for i, rs := range group.inputs {
						want := legacy.receive(t, rs)
						got := v1.receive(t, rs)
						for _, e := range otlpPipelineParityExceptions {
							if e.divergentOnly && !group.divergent {
								continue
							}
							for j := range want.payloads {
								if j < len(got.payloads) {
									e.apply(legacy.agnt.conf(), rs, &want.payloads[j], &got.payloads[j])
								}
							}
						}
						compareOTLPParityRuns(t, fmt.Sprintf("input %d", i), want, got)
					}
					if !group.divergent {
						var gits []string
						for _, rs := range group.inputs {
							gits = append(gits, inputAttributeValues(rs, "_dd.git.commit.sha")...)
						}
						compareOTLPParityStats(t, legacy.concentrator.Flush(true), v1.concentrator.Flush(true), gits)
					}
					assert.Equal(t, legacy.statsd.counts(), v1.statsd.counts(), "OTLP receiver telemetry")
				})
			}
		})
	}
}

// withResourceEnvAndVersion returns a copy of rs where the resource has the
// given env and version, and so do the spans that set an env or a version. The
// spans that set a git commit SHA or an APM mode, which are promoted to the
// payload too, are also given one of the resource.
func withResourceEnvAndVersion(rs ptrace.ResourceSpans, env, version string) ptrace.ResourceSpans {
	out := ptrace.NewResourceSpans()
	rs.CopyTo(out)
	out.Resource().Attributes().PutStr("deployment.environment.name", env)
	out.Resource().Attributes().PutStr("service.version", version)
	for i := 0; i < out.ScopeSpans().Len(); i++ {
		spans := out.ScopeSpans().At(i).Spans()
		for j := 0; j < spans.Len(); j++ {
			attrs := spans.At(j).Attributes()
			for _, k := range []string{"env", "deployment.environment", "deployment.environment.name"} {
				if _, ok := attrs.Get(k); ok {
					attrs.PutStr(k, env)
				}
			}
			for _, k := range []string{"version", "service.version"} {
				if _, ok := attrs.Get(k); ok {
					attrs.PutStr(k, version)
				}
			}
			if _, ok := attrs.Get("_dd.git.commit.sha"); ok {
				attrs.PutStr("_dd.git.commit.sha", "parity-sha-"+version)
			}
			if _, ok := attrs.Get("_dd.apm_mode"); ok {
				attrs.PutStr("_dd.apm_mode", "edge")
			}
		}
	}
	return out
}

// otlpParityResourceSpans returns hand-written inputs covering what the
// dupcheck corpus doesn't: 128-bit trace IDs, sampling priorities and decision
// makers, errors, numeric and typed attributes, span events and links.
func otlpParityResourceSpans() []ptrace.ResourceSpans {
	now := time.Now().UnixNano()
	newSpan := func(ss ptrace.ScopeSpans, traceHigh, traceLow, spanID, parentID uint64, kind ptrace.SpanKind) ptrace.Span {
		s := ss.Spans().AppendEmpty()
		var tid [16]byte
		binary.BigEndian.PutUint64(tid[:8], traceHigh)
		binary.BigEndian.PutUint64(tid[8:], traceLow)
		s.SetTraceID(tid)
		s.SetSpanID(paritySpanID(spanID))
		if parentID != 0 {
			s.SetParentSpanID(paritySpanID(parentID))
		}
		s.SetKind(kind)
		s.SetName("parity-" + strconv.FormatUint(spanID, 10))
		s.SetStartTimestamp(pcommon.Timestamp(now - 1000))
		s.SetEndTimestamp(pcommon.Timestamp(now))
		return s
	}

	var inputs []ptrace.ResourceSpans

	// HTTP server and client spans with 128-bit trace IDs, an error, and an
	// upstream sampling decision.
	rs := ptrace.NewResourceSpans()
	rattrs := rs.Resource().Attributes()
	rattrs.PutStr("service.name", "parity-web")
	rattrs.PutStr("deployment.environment.name", "parity-env")
	rattrs.PutStr("service.version", "1.2.3")
	rattrs.PutStr("telemetry.sdk.language", "go")
	rattrs.PutStr("telemetry.sdk.version", "1.30.0")
	rattrs.PutStr("container.id", "parity-container")
	rattrs.PutStr("k8s.pod.name", "parity-pod")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("parity-scope")
	ss.Scope().SetVersion("0.1.0")
	root := newSpan(ss, 0x1122334455667788, 1, 10, 0, ptrace.SpanKindServer)
	root.Attributes().PutStr("http.request.method", "GET")
	root.Attributes().PutStr("http.route", "/users/:id")
	root.Attributes().PutInt("http.response.status_code", 500)
	root.Attributes().PutInt("sampling.priority", 2)
	root.Attributes().PutStr("_dd.p.dm", "-3")
	root.Status().SetCode(ptrace.StatusCodeError)
	ev := root.Events().AppendEmpty()
	ev.SetName("exception")
	ev.SetTimestamp(pcommon.Timestamp(now - 500))
	ev.Attributes().PutStr("exception.message", "boom")
	ev.Attributes().PutInt("attempt", 3)
	ev.Attributes().PutBool("handled", false)
	arr := ev.Attributes().PutEmptySlice("codes")
	arr.AppendEmpty().SetInt(1)
	arr.AppendEmpty().SetInt(2)
	mixed := ev.Attributes().PutEmptySlice("mixed")
	mixed.AppendEmpty().SetStr("a")
	mixed.AppendEmpty().SetDouble(1.5)
	child := newSpan(ss, 0x1122334455667788, 1, 11, 10, ptrace.SpanKindClient)
	child.Attributes().PutStr("db.system", "postgresql")
	child.Attributes().PutStr("db.statement", "SELECT * FROM users WHERE id = 42")
	child.Attributes().PutStr("peer.service", "parity-db")
	child.Attributes().PutDouble("custom.ratio", 0.25)
	child.TraceState().FromRaw("dd=s:2;t.dm:-12")
	link := child.Links().AppendEmpty()
	var linkTID [16]byte
	binary.BigEndian.PutUint64(linkTID[:8], 0xaabbccdd)
	binary.BigEndian.PutUint64(linkTID[8:], 77)
	link.SetTraceID(linkTID)
	link.SetSpanID(paritySpanID(78))
	link.SetFlags(1)
	link.TraceState().FromRaw("dd=s:1")
	link.Attributes().PutStr("link.kind", "follows")
	link.Attributes().PutEmptyMap("nested").PutStr("k", "v")
	inputs = append(inputs, rs)

	// Messaging and RPC spans without sampling priorities, from two scopes and
	// two traces, with numeric attributes colliding with resource attributes.
	rs = ptrace.NewResourceSpans()
	rattrs = rs.Resource().Attributes()
	rattrs.PutStr("service.name", "parity-worker")
	rattrs.PutStr("collision", "from-resource")
	rattrs.PutStr("_dd.hostname", "parity-host")
	ss = rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("parity-messaging")
	ss.Scope().Attributes().PutStr("scope.attr", "scope-value")
	consumer := newSpan(ss, 0xdeadbeef, 2, 20, 0, ptrace.SpanKindConsumer)
	consumer.Attributes().PutStr("messaging.system", "kafka")
	consumer.Attributes().PutStr("messaging.operation", "process")
	consumer.Attributes().PutStr("messaging.destination.name", "orders")
	consumer.Attributes().PutInt("collision", 7)
	producer := newSpan(ss, 0xdeadbeef, 2, 21, 20, ptrace.SpanKindProducer)
	producer.Attributes().PutStr("messaging.system", "kafka")
	producer.Attributes().PutStr("messaging.operation", "publish")
	producer.Attributes().PutInt("_dd.measured", 1)
	ss = rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("parity-rpc")
	rpc := newSpan(ss, 0, 3, 30, 0, ptrace.SpanKindClient)
	rpc.Attributes().PutStr("rpc.system", "grpc")
	rpc.Attributes().PutStr("rpc.service", "Users")
	rpc.Attributes().PutStr("rpc.method", "Get")
	rpc.Attributes().PutInt("rpc.grpc.status_code", 2)
	rpc.Attributes().PutStr("analytics.event", "true")
	rpc.Status().SetCode(ptrace.StatusCodeError)
	rpc.Status().SetMessage("unavailable")
	internal := newSpan(ss, 0, 3, 31, 30, ptrace.SpanKindInternal)
	internal.Attributes().PutStr("component", "parity-component")
	internal.Attributes().PutDouble("_sample_rate", 0.5)
	internal.Attributes().PutInt("sampling.priority", 0)
	inputs = append(inputs, rs)
	return inputs
}

// otlpParityAgent is an agent built by NewAgent, sending to a dupcheckIntake,
// whose OTLP receiver feeds its V1 pipeline, or the legacy one if legacy.
type otlpParityAgent struct {
	agnt         parityAgent
	intake       *dupcheckIntake
	legacy       bool
	concentrator *stats.Concentrator
	statsd       *countingStatsd
}

func newOTLPParityAgent(t *testing.T, c dupcheckConfig, legacy bool, now time.Time) *otlpParityAgent {
	intake := newDupcheckIntake(t)
	cfg := config.New()
	cfg.Endpoints[0].Host = intake.server.URL
	cfg.Endpoints[0].APIKey = "dupcheck-api-key"
	cfg.SynchronousFlushing = true
	cfg.Hostname = "dupcheck-agent-hostname"
	cfg.Obfuscation.Cache.Enabled = false
	cfg.DefaultEnv = "dupcheck-agent-env"
	translator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	cfg.OTLPReceiver.AttributesTranslator = translator
	if c.configure != nil {
		c.configure(cfg)
	}
	if legacy {
		cfg.Features["disable-convert-traces"] = struct{}{}
	}
	// receive reads the payload from Agent.In or Agent.InV1 after the receiver sends it.
	cfg.TraceBuffer = 1
	// The samplers update their rates on wall clock buckets of a few seconds. Keep
	// them from lowering rates so that their decisions and the rates they set on
	// spans don't depend on when each agent crosses a bucket boundary.
	cfg.TargetTPS = 1e9
	cfg.ErrorTPS = 1e9
	cfg.MaxEPS = 1e9
	sd := &countingStatsd{}
	a := agent.NewAgent(context.Background(), cfg, telemetry.NewNoopCollector(), sd, gzipcomp.NewComponent())
	conc := stats.NewConcentrator(cfg, noopStatsWriter{}, now, &statsd.NoOpClient{})
	a.Concentrator = conc
	go a.StatsWriter.Run()
	if c.setup != nil {
		c.setup(a)
	}
	t.Cleanup(func() {
		a.TraceWriter.Stop()
		a.TraceWriterV1.Stop()
		a.StatsWriter.Stop()
	})
	return &otlpParityAgent{agnt: parityAgent{Agent: a, cfg: cfg}, intake: intake, legacy: legacy, concentrator: conc, statsd: sd}
}

// parityAgent is an agent with its configuration.
type parityAgent struct {
	*agent.Agent
	cfg *config.AgentConfig
}

func (a parityAgent) conf() *config.AgentConfig { return a.cfg }

// otlpParityRun is what an agent sent for an OTLP input.
type otlpParityRun struct {
	source   info.Tags
	stats    *info.Stats
	agent    []otlpParityAgentPayload
	payloads []otlpParityPayload
}

// receive sends rs to the OTLP receiver of the agent, processes the payload
// the receiver sends to the agent and returns what the agent sent.
func (p *otlpParityAgent) receive(t *testing.T, rs ptrace.ResourceSpans) otlpParityRun {
	received := ptrace.NewResourceSpans()
	rs.CopyTo(received)
	_, err := p.agnt.Agent.OTLPReceiver.ReceiveResourceSpans(context.Background(), received, http.Header{}, nil)
	require.NoError(t, err)
	var source *info.TagStats
	if p.legacy {
		require.Empty(t, p.agnt.InV1, "the legacy pipeline must not get V1 payloads")
		in := <-p.agnt.In
		source = in.Source
		p.agnt.Process(in)
		require.NoError(t, p.agnt.TraceWriter.FlushSync())
	} else {
		require.Empty(t, p.agnt.In, "the V1 pipeline must not get legacy payloads")
		in := <-p.agnt.InV1
		source = in.Source
		p.agnt.ProcessV1(in)
		require.NoError(t, p.agnt.TraceWriterV1.FlushSync())
	}
	run := otlpParityRun{source: source.Tags, stats: &source.Stats}
	for _, ap := range p.intake.takeTraces(t) {
		if p.legacy {
			require.Empty(t, ap.IdxTracerPayloads)
		} else {
			require.Empty(t, ap.TracerPayloads)
		}
		run.agent = append(run.agent, otlpParityAgentPayloadOf(ap))
		for _, tp := range ap.TracerPayloads {
			run.payloads = append(run.payloads, legacyParityPayload(tp))
		}
		for _, tp := range ap.IdxTracerPayloads {
			run.payloads = append(run.payloads, v1ParityPayload(idx.FromProto(tp)))
		}
	}
	return run
}

// Normalized views of the outbound payloads, common to the legacy and V1
// formats. Attribute values are prefixed by their type: "s:" for strings,
// "d:" for doubles, "i:" for ints, "b:" for bools.

type otlpParityAgentPayload struct {
	hostName, env, agentVersion string
	targetTPS, errorTPS         float64
	rareSamplerEnabled          bool
	tags                        map[string]string
}

type otlpParityPayload struct {
	hostname, env, appVersion, containerID   string
	languageName, languageVersion, tracerVer string
	runtimeID                                string
	attributes                               map[string]string
	chunks                                   map[string]otlpParityChunk // by trace ID
}

type otlpParityChunk struct {
	priority          int32
	origin            string
	droppedTrace      bool
	samplingMechanism uint32
	attributes        map[string]string
	spans             map[uint64]otlpParitySpan // by span ID
}

type otlpParitySpan struct {
	service, name, resource, typ  string
	env, version, component, kind string
	parentID                      uint64
	start, duration               int64
	err                           bool
	attributes                    map[string]string
	events                        []otlpParityEvent
	links                         []otlpParityLink
}

type otlpParityEvent struct {
	time       uint64
	name       string
	attributes map[string]string
}

type otlpParityLink struct {
	traceID    string
	spanID     uint64
	tracestate string
	flags      uint32
	attributes map[string]string
}

func otlpParityAgentPayloadOf(ap *pb.AgentPayload) otlpParityAgentPayload {
	return otlpParityAgentPayload{
		hostName:           ap.HostName,
		env:                ap.Env,
		agentVersion:       ap.AgentVersion,
		targetTPS:          ap.TargetTPS,
		errorTPS:           ap.ErrorTPS,
		rareSamplerEnabled: ap.RareSamplerEnabled,
		tags:               ap.Tags,
	}
}

func stringsAsAttributes(m map[string]string) map[string]string {
	attrs := make(map[string]string, len(m))
	for k, v := range m {
		attrs[k] = "s:" + v
	}
	return attrs
}

// legacyParityPayload normalizes a legacy tracer payload the way ConvertToIdx
// maps it to V1.
func legacyParityPayload(tp *pb.TracerPayload) otlpParityPayload {
	p := otlpParityPayload{
		hostname:        tp.Hostname,
		env:             tp.Env,
		appVersion:      tp.AppVersion,
		containerID:     tp.ContainerID,
		languageName:    tp.LanguageName,
		languageVersion: tp.LanguageVersion,
		tracerVer:       tp.TracerVersion,
		runtimeID:       tp.RuntimeID,
		attributes:      normalizeContainerTags(stringsAsAttributes(tp.Tags)),
		chunks:          make(map[string]otlpParityChunk),
	}
	for _, c := range tp.Chunks {
		if len(c.Spans) == 0 {
			continue
		}
		upper, lower, _ := c.Spans[0].Get128BitTraceID()
		chunk := otlpParityChunk{
			priority:     c.Priority,
			origin:       c.Origin,
			droppedTrace: c.DroppedTrace,
			attributes:   stringsAsAttributes(c.Tags),
			spans:        make(map[uint64]otlpParitySpan),
		}
		if dm, ok := c.Tags["_dd.p.dm"]; ok {
			chunk.samplingMechanism = parseDecisionMaker(dm)
		}
		for _, s := range c.Spans {
			if dm, ok := s.Meta["_dd.p.dm"]; ok && chunk.samplingMechanism == 0 {
				chunk.samplingMechanism = parseDecisionMaker(dm)
			}
			chunk.spans[s.SpanID] = legacyParitySpan(s)
		}
		p.chunks[traceIDKey(upper, lower)] = chunk
	}
	return p
}

func parseDecisionMaker(dm string) uint32 {
	dm, _ = strings.CutPrefix(dm, "-")
	v, _ := strconv.ParseUint(dm, 10, 32)
	return uint32(v)
}

func traceIDKey(upper, lower uint64) string {
	return fmt.Sprintf("%016x%016x", upper, lower)
}

func legacyParitySpan(s *pb.Span) otlpParitySpan {
	ps := otlpParitySpan{
		service:    s.Service,
		name:       s.Name,
		resource:   s.Resource,
		typ:        s.Type,
		env:        s.Meta["env"],
		version:    s.Meta["version"],
		component:  s.Meta["component"],
		kind:       idxKindName(s.Meta["span.kind"]),
		parentID:   s.ParentID,
		start:      s.Start,
		duration:   s.Duration,
		err:        s.Error > 0,
		attributes: make(map[string]string, len(s.Meta)+len(s.Metrics)),
	}
	for k, v := range s.Meta {
		if !isPromotedKey(k) {
			ps.attributes[k] = "s:" + v
		}
	}
	for k, v := range s.Metrics {
		if !isPromotedKey(k) {
			ps.attributes[k] = "d:" + strconv.FormatFloat(v, 'g', -1, 64)
		}
	}
	for k, v := range s.MetaStruct {
		ps.attributes[k] = "bytes:" + hex.EncodeToString(v)
	}
	for _, e := range s.SpanEvents {
		pe := otlpParityEvent{time: e.TimeUnixNano, name: e.Name, attributes: make(map[string]string, len(e.Attributes))}
		for k, v := range e.Attributes {
			pe.attributes[k] = legacyEventValue(v)
		}
		ps.events = append(ps.events, pe)
	}
	for _, l := range s.SpanLinks {
		ps.links = append(ps.links, otlpParityLink{
			traceID:    traceIDKey(l.TraceIDHigh, l.TraceID),
			spanID:     l.SpanID,
			tracestate: l.Tracestate,
			flags:      l.Flags,
			attributes: stringsAsAttributes(l.Attributes),
		})
	}
	return ps
}

func legacyEventValue(v *pb.AttributeAnyValue) string {
	switch v.Type {
	case pb.AttributeAnyValue_STRING_VALUE:
		return "s:" + v.StringValue
	case pb.AttributeAnyValue_BOOL_VALUE:
		return "b:" + strconv.FormatBool(v.BoolValue)
	case pb.AttributeAnyValue_INT_VALUE:
		return "i:" + strconv.FormatInt(v.IntValue, 10)
	case pb.AttributeAnyValue_DOUBLE_VALUE:
		return "d:" + strconv.FormatFloat(v.DoubleValue, 'g', -1, 64)
	case pb.AttributeAnyValue_ARRAY_VALUE:
		var values []string
		for _, av := range v.ArrayValue.GetValues() {
			values = append(values, legacyEventValue(&pb.AttributeAnyValue{
				Type:        pb.AttributeAnyValue_AttributeAnyValueType(av.Type),
				StringValue: av.StringValue,
				BoolValue:   av.BoolValue,
				IntValue:    av.IntValue,
				DoubleValue: av.DoubleValue,
			}))
		}
		return "a:[" + strings.Join(values, ",") + "]"
	}
	return fmt.Sprintf("unknown:%v", v)
}

func isPromotedKey(k string) bool {
	return k == "env" || k == "version" || k == "component" || k == "span.kind"
}

// idxKindName returns the name of the V1 span kind that ConvertToIdx gives to
// the legacy span.kind tag k.
func idxKindName(k string) string {
	switch k {
	case "server", "client", "producer", "consumer":
		return k
	}
	return "internal"
}

func v1ParityPayload(tp *idx.InternalTracerPayload) otlpParityPayload {
	p := otlpParityPayload{
		hostname:        tp.Hostname(),
		env:             tp.Env(),
		appVersion:      tp.AppVersion(),
		containerID:     tp.ContainerID(),
		languageName:    tp.LanguageName(),
		languageVersion: tp.LanguageVersion(),
		tracerVer:       tp.TracerVersion(),
		runtimeID:       tp.RuntimeID(),
		attributes:      normalizeContainerTags(v1Attributes(tp.Strings, tp.Attributes)),
		chunks:          make(map[string]otlpParityChunk),
	}
	for _, c := range tp.Chunks {
		chunk := otlpParityChunk{
			priority:          c.Priority,
			origin:            c.Origin(),
			droppedTrace:      c.DroppedTrace,
			samplingMechanism: c.SamplingMechanism(),
			attributes:        v1Attributes(c.Strings, c.Attributes),
			spans:             make(map[uint64]otlpParitySpan),
		}
		for _, s := range c.Spans {
			chunk.spans[s.SpanID()] = v1ParitySpan(s)
		}
		p.chunks[hex.EncodeToString(c.TraceID)] = chunk
	}
	return p
}

func v1Value(strs *idx.StringTable, v *idx.AnyValue) string {
	switch v := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return "s:" + strs.Get(v.StringValueRef)
	case *idx.AnyValue_BoolValue:
		return "b:" + strconv.FormatBool(v.BoolValue)
	case *idx.AnyValue_IntValue:
		return "i:" + strconv.FormatInt(v.IntValue, 10)
	case *idx.AnyValue_DoubleValue:
		return "d:" + strconv.FormatFloat(v.DoubleValue, 'g', -1, 64)
	case *idx.AnyValue_BytesValue:
		return "bytes:" + hex.EncodeToString(v.BytesValue)
	case *idx.AnyValue_ArrayValue:
		var values []string
		for _, av := range v.ArrayValue.GetValues() {
			values = append(values, v1Value(strs, av))
		}
		return "a:[" + strings.Join(values, ",") + "]"
	}
	return fmt.Sprintf("unknown:%v", v)
}

func v1Attributes(strs *idx.StringTable, attrs map[uint32]*idx.AnyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for k, v := range attrs {
		m[strs.Get(k)] = v1Value(strs, v)
	}
	return m
}

func v1ParitySpan(s *idx.InternalSpan) otlpParitySpan {
	ps := otlpParitySpan{
		service:    s.Service(),
		name:       s.Name(),
		resource:   s.Resource(),
		typ:        s.Type(),
		env:        s.Env(),
		version:    s.Version(),
		component:  s.Component(),
		kind:       s.SpanKind(),
		parentID:   s.ParentID(),
		start:      int64(s.Start()),
		duration:   int64(s.Duration()),
		err:        s.Error(),
		attributes: v1Attributes(s.Strings, s.Attributes()),
	}
	for _, e := range s.Events() {
		ps.events = append(ps.events, otlpParityEvent{time: e.Time(), name: e.Name(), attributes: v1Attributes(s.Strings, e.Attributes())})
	}
	for _, l := range s.Links() {
		ps.links = append(ps.links, otlpParityLink{
			traceID:    hex.EncodeToString(l.TraceID()),
			spanID:     l.SpanID(),
			tracestate: l.Tracestate(),
			flags:      l.Flags(),
			attributes: v1Attributes(s.Strings, l.Attributes()),
		})
	}
	return ps
}

// normalizeContainerTags sorts the container tags of the payload attributes:
// both pipelines flatten them from a map, in a random order.
func normalizeContainerTags(attrs map[string]string) map[string]string {
	if v, ok := attrs["_dd.tags.container"]; ok {
		tags := strings.Split(strings.TrimPrefix(v, "s:"), ",")
		slices.Sort(tags)
		attrs["_dd.tags.container"] = "s:" + strings.Join(tags, ",")
	}
	return attrs
}

// otlpPipelineParityException is an expected difference between the payloads
// sent by the legacy (want) and the V1 (got) pipelines for the same OTLP input.
type otlpPipelineParityException struct {
	// reason explains why the difference is expected.
	reason string
	// divergentOnly restricts the exception to the divergent inputs.
	divergentOnly bool
	// apply removes the expected difference from want and got, for the
	// configuration cfg of the agents.
	apply func(cfg *config.AgentConfig, in ptrace.ResourceSpans, want, got *otlpParityPayload)
}

// forEachChunk calls f for each chunk of want with the chunk of the same trace
// in got, and stores the chunks f changed.
func forEachChunk(want, got *otlpParityPayload, f func(want, got *otlpParityChunk)) {
	for tid, wc := range want.chunks {
		gc, ok := got.chunks[tid]
		if !ok {
			continue
		}
		f(&wc, &gc)
		want.chunks[tid], got.chunks[tid] = wc, gc
	}
}

// inputAttributeValues returns the values of the attributes keys of the spans,
// resource and scopes of the OTLP input, raw and normalized as tag values.
func inputAttributeValues(in ptrace.ResourceSpans, keys ...string) []string {
	var values []string
	add := func(attrs pcommon.Map) {
		for _, k := range keys {
			if v, ok := attrs.Get(k); ok {
				values = append(values, v.AsString(), normalize.NormalizeTagValue(v.AsString()))
			}
		}
	}
	add(in.Resource().Attributes())
	for i := 0; i < in.ScopeSpans().Len(); i++ {
		ss := in.ScopeSpans().At(i)
		add(ss.Scope().Attributes())
		for j := 0; j < ss.Spans().Len(); j++ {
			add(ss.Spans().At(j).Attributes())
		}
	}
	return values
}

// rootSpan returns the root span of the chunk, if any.
func rootSpan(c *otlpParityChunk) (otlpParitySpan, bool) {
	for _, s := range c.spans {
		if s.parentID == 0 {
			return s, true
		}
	}
	return otlpParitySpan{}, false
}

// otlpPipelineParityExceptions is the reviewed list of expected differences
// between the legacy and V1 pipelines. Anything else fails the test. These are
// differences between Agent.Process and Agent.ProcessV1 and between the legacy
// and V1 payload formats, which every V1 payload has, whatever its source: the
// OTLP conversion itself is checked to give exactly ConvertToIdx's output by
// the parity tests of pkg/trace/api.
var otlpPipelineParityExceptions = []otlpPipelineParityException{
	{
		reason: "The legacy pipeline sends the sampling decision maker as the _dd.p.dm chunk tag; " +
			"the V1 format has a sampling mechanism chunk field instead, compared on its own.",
		apply: func(_ *config.AgentConfig, _ ptrace.ResourceSpans, want, got *otlpParityPayload) {
			forEachChunk(want, got, func(w, g *otlpParityChunk) {
				if dm, ok := w.attributes["_dd.p.dm"]; ok && parseDecisionMaker(strings.TrimPrefix(dm, "s:")) == g.samplingMechanism {
					w.attributes = maps.Clone(w.attributes)
					delete(w.attributes, "_dd.p.dm")
				}
			})
		},
	},
	{
		reason: "The chunk origin is taken from the _dd.origin tag of the root span by Process, " +
			"and from the first span that has one by the conversion to V1 (as ConvertToIdx does for all V1 payloads).",
		apply: func(_ *config.AgentConfig, _ ptrace.ResourceSpans, want, got *otlpParityPayload) {
			forEachChunk(want, got, func(w, g *otlpParityChunk) {
				if w.origin == g.origin {
					return
				}
				root, _ := rootSpan(w)
				rootOrigin := strings.TrimPrefix(root.attributes["_dd.origin"], "s:")
				if w.origin != rootOrigin {
					return
				}
				for _, s := range g.spans {
					if strings.TrimPrefix(s.attributes["_dd.origin"], "s:") == g.origin {
						w.origin = g.origin
						return
					}
				}
			})
		},
	},
	{
		reason: "The V1 conversion promotes the _dd.git.commit.sha and _dd.apm_mode tags of the spans to payload attributes " +
			"(ConvertToIdx); Process only sets the _dd.apm_mode payload tag, from the root span of the first chunk.",
		apply: func(_ *config.AgentConfig, in ptrace.ResourceSpans, want, got *otlpParityPayload) {
			for _, k := range []string{"_dd.git.commit.sha", "_dd.apm_mode"} {
				wv, wok := want.attributes[k]
				gv, gok := got.attributes[k]
				if wok == gok && wv == gv {
					continue
				}
				spanValues := inputAttributeValues(in, k)
				// The V1 payload must carry the key whenever an input span carries it; only the
				// legacy payload may omit it (Process sets _dd.apm_mode from the root span of the
				// first chunk only, and _dd.git.commit.sha from the container tags). Accepting a
				// missing V1 key here would hide a regression that stops promoting it.
				if (!wok || slices.Contains(spanValues, strings.TrimPrefix(wv, "s:"))) && gok && slices.Contains(spanValues, strings.TrimPrefix(gv, "s:")) {
					want.attributes, got.attributes = maps.Clone(want.attributes), maps.Clone(got.attributes)
					delete(want.attributes, k)
					delete(got.attributes, k)
				}
			}
		},
	},
	{
		reason: "Divergent inputs only: Process sets the payload env and app version from the root span of the first chunk, " +
			"the V1 conversion from the spans of the last chunk that has them (ConvertToIdx).",
		divergentOnly: true,
		apply: func(_ *config.AgentConfig, in ptrace.ResourceSpans, want, got *otlpParityPayload) {
			envs := inputAttributeValues(in, "env", "deployment.environment", "deployment.environment.name")
			versions := inputAttributeValues(in, "version", "service.version")
			if want.env != got.env && slices.Contains(envs, want.env) && slices.Contains(envs, got.env) {
				want.env = got.env
			}
			if want.appVersion != got.appVersion && slices.Contains(versions, want.appVersion) && slices.Contains(versions, got.appVersion) {
				want.appVersion = got.appVersion
			}
		},
	},
}

func compareOTLPParityRuns(t *testing.T, context string, want, got otlpParityRun) {
	t.Helper()
	assert.Equal(t, want.source, got.source, "%s: receiver tag stats tags", context)
	assert.Equal(t, want.stats, got.stats, "%s: receiver tag stats counters", context)
	require.Equal(t, len(want.agent), len(got.agent), "%s: number of agent payloads", context)
	for i := range want.agent {
		assert.Equal(t, want.agent[i], got.agent[i], "%s: agent payload %d", context, i)
	}
	require.Equal(t, len(want.payloads), len(got.payloads), "%s: number of tracer payloads", context)
	for i := range want.payloads {
		wp, gp := want.payloads[i], got.payloads[i]
		wchunks, gchunks := wp.chunks, gp.chunks
		wp.chunks, gp.chunks = nil, nil
		assert.Equal(t, wp, gp, "%s: tracer payload %d", context, i)
		assert.ElementsMatch(t, slices.Collect(maps.Keys(wchunks)), slices.Collect(maps.Keys(gchunks)), "%s: chunk trace IDs of tracer payload %d", context, i)
		for tid, wc := range wchunks {
			gc, ok := gchunks[tid]
			if !ok {
				continue
			}
			wspans, gspans := wc.spans, gc.spans
			wc.spans, gc.spans = nil, nil
			assert.Equal(t, wc, gc, "%s: chunk %s", context, tid)
			assert.ElementsMatch(t, slices.Collect(maps.Keys(wspans)), slices.Collect(maps.Keys(gspans)), "%s: span IDs of chunk %s", context, tid)
			for id, ws := range wspans {
				gs, ok := gspans[id]
				if !ok {
					continue
				}
				assert.Equal(t, ws, gs, "%s: span %d of chunk %s", context, id, tid)
			}
		}
	}
}

// compareOTLPParityStats compares the APM stats computed by both pipelines,
// ignoring the order of the stats. spanGits are the _dd.git.commit.sha
// attributes of the inputs.
//
// Expected difference: ProcessV1 takes the git commit SHA of the stats from the
// _dd.git.commit.sha payload attribute, which the V1 conversion promotes from
// the spans (as ConvertToIdx does for all V1 payloads), before the container
// tags; Process only takes it from the container tags. So the git commit SHA of
// the V1 stats must be the legacy one or one of spanGits, and is otherwise
// left out of the comparison.
//
// Expected difference: the V1 span kind is an enum, where a span.kind tag that
// isn't a known kind is internal (ConvertToIdx). The legacy OTLP receiver
// (disable_receive_resource_spans_v2) lets span attributes overwrite span.kind,
// which the legacy stats keep as is, so the legacy span kinds are mapped like
// ConvertToIdx does.
func compareOTLPParityStats(t *testing.T, want, got *pb.StatsPayload, spanGits []string) {
	t.Helper()
	require.NotNil(t, want)
	require.NotNil(t, got)
	// normalize below returns equal (empty) slices when neither pipeline produced stats, so
	// without these the comparison would pass while comparing nothing.
	require.NotEmpty(t, want.Stats, "legacy pipeline produced no APM stats to compare")
	require.NotEmpty(t, got.Stats, "V1 pipeline produced no APM stats to compare")
	wantGits := make(map[string]bool)
	for _, csp := range want.Stats {
		wantGits[csp.GitCommitSha] = true
	}
	for _, csp := range got.Stats {
		if !wantGits[csp.GitCommitSha] && !slices.Contains(spanGits, csp.GitCommitSha) {
			t.Errorf("APM stats: unexpected V1 git commit SHA %q", csp.GitCommitSha)
		}
	}
	normalize := func(p *pb.StatsPayload, mapKind func(string) string) []string {
		var out []string
		for _, csp := range p.Stats {
			header := fmt.Sprintf("host=%s env=%s version=%s cid=%s ctags=%v image=%s lang=%s tracer=%s ptags=%s",
				csp.Hostname, csp.Env, csp.Version, csp.ContainerID, csp.Tags, csp.ImageTag, csp.Lang, csp.TracerVersion, csp.ProcessTags)
			for _, b := range csp.Stats {
				for _, g := range b.Stats {
					okSummary := len(g.OkSummary)
					errSummary := len(g.ErrorSummary)
					out = append(out, fmt.Sprintf("%s start=%d dur=%d svc=%s name=%s res=%s type=%s http=%d kind=%s peer=%v synth=%v toplevel=%d hits=%d errors=%d duration=%d trace_root=%v grpc=%s method=%s endpoint=%s ok_sketch=%d err_sketch=%d",
						header, b.Start, b.Duration, g.Service, g.Name, g.Resource, g.Type, g.HTTPStatusCode, mapKind(g.SpanKind), g.PeerTags, g.Synthetics, g.TopLevelHits, g.Hits, g.Errors, g.Duration, g.IsTraceRoot, g.GRPCStatusCode, g.HTTPMethod, g.HTTPEndpoint, okSummary, errSummary))
				}
			}
		}
		slices.Sort(out)
		return out
	}
	assert.Equal(t, normalize(want, idxKindName), normalize(got, func(k string) string { return k }), "APM stats")
}

type noopStatsWriter struct{}

func (noopStatsWriter) Write(*pb.StatsPayload) {}

// countingStatsd records the OTLP receiver counts.
type countingStatsd struct {
	statsd.NoOpClient
	mu sync.Mutex
	c  map[string]int64
}

func (s *countingStatsd) Count(name string, value int64, tags []string, _ float64) error {
	if !strings.HasPrefix(name, "datadog.trace_agent.otlp.") {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c == nil {
		s.c = make(map[string]int64)
	}
	tags = slices.Clone(tags)
	slices.Sort(tags)
	s.c[name+" "+strings.Join(tags, ",")] += value
	return nil
}

func (s *countingStatsd) counts() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.c)
}

func paritySpanID(id uint64) pcommon.SpanID {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], id)
	return pcommon.SpanID(b)
}
