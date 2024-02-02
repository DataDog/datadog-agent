// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APM) Fix revive linter
package agent

import (
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	// tagHostname specifies the hostname of the tracer.
	// DEPRECATED: Tracer hostname is now specified as a TracerPayload field.
	tagHostname = "_dd.hostname"

	// tagInstallID, tagInstallType, and tagInstallTime are included in the first trace sent by the agent,
	// and used to track successful onboarding onto APM.
	tagInstallID   = "_dd.install.id"
	tagInstallType = "_dd.install.type"
	tagInstallTime = "_dd.install.time"

	// manualSampling is the value for _dd.p.dm when user sets sampling priority directly in code.
	manualSampling = "-4"

	// tagDecisionMaker specifies the sampling decision maker
	tagDecisionMaker = "_dd.p.dm"
)

const (
	tagRedisRawCommand  = "redis.raw_command"
	tagMemcachedCommand = "memcached.command"
	tagMongoDBQuery     = "mongodb.query"
	tagElasticBody      = "elasticsearch.body"
	tagSQLQuery         = "sql.query"
	tagHTTPURL          = "http.url"
)

const (
	textNonParsable = "Non-parsable SQL query"
)

// ccObfuscator maintains credit card obfuscation state and processing.
type ccObfuscator struct {
	luhn bool
}

func (cco *ccObfuscator) Stop() { pb.SetMetaHook(nil) }

// MetaHook checks the tag with the given key and val and returns the final
// value to be assigned to this tag.
//
// For example, in this specific use-case, if the val is detected to be a credit
// card number, "?" will be returned.
func (cco *ccObfuscator) MetaHook(k, v string) (newval string) {
	switch k {
	case "_sample_rate",
		"_sampling_priority_v1",
		"error",
		"error.msg",
		"error.type",
		"error.stack",
		"env",
		"graphql.field",
		"graphql.query",
		"graphql.type",
		"graphql.operation.name",
		"grpc.code",
		"grpc.method",
		"grpc.request",
		"http.status_code",
		"http.method",
		"runtime-id",
		"out.host",
		"out.port",
		"sampling.priority",
		"span.type",
		"span.name",
		"service.name",
		"service",
		"sql.query",
		"version":
		// these tags are known to not be credit card numbers
		return v
	}
	if strings.HasPrefix(k, "_dd") {
		return v
	}
	if obfuscate.IsCardNumber(v, cco.luhn) {
		return "?"
	}
	return v
}

const (
	// MaxTypeLen the maximum length a span type can have
	MaxTypeLen = 100
	// tagOrigin specifies the origin of the trace.
	// DEPRECATED: Origin is now specified as a TraceChunk field.
	tagOrigin = "_dd.origin"
	// tagSamplingPriority specifies the sampling priority of the trace.
	// DEPRECATED: Priority is now specified as a TraceChunk field.
	tagSamplingPriority = "_sampling_priority_v1"
	// peerServiceKey is the key for the peer.service meta field.
	peerServiceKey = "peer.service"
)

var (
	// Year2000NanosecTS is an arbitrary cutoff to spot weird-looking values
	Year2000NanosecTS = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
)

const (
	// MaxMetaKeyLen the maximum length of metadata key
	MaxMetaKeyLen = 200
	// MaxMetaValLen the maximum length of metadata value
	MaxMetaValLen = 25000
	// MaxMetricsKeyLen the maximum length of a metric name key
	MaxMetricsKeyLen = MaxMetaKeyLen
)

func traceContainsError(trace pb.Trace) bool {
	for _, span := range trace {
		if span.Error != 0 {
			return true
		}
	}
	return false
}

func filteredByTags(root *pb.Span, require, reject []*config.Tag, requireRegex, rejectRegex []*config.TagRegex) bool {
	for _, tag := range reject {
		if v, ok := root.Meta[tag.K]; ok && (tag.V == "" || v == tag.V) {
			return true
		}
	}
	for _, tag := range rejectRegex {
		if v, ok := root.Meta[tag.K]; ok && (tag.V == nil || tag.V.MatchString(v)) {
			return true
		}
	}
	for _, tag := range require {
		v, ok := root.Meta[tag.K]
		if !ok || (tag.V != "" && v != tag.V) {
			return true
		}
	}
	for _, tag := range requireRegex {
		v, ok := root.Meta[tag.K]
		if !ok || (tag.V != nil && !tag.V.MatchString(v)) {
			return true
		}
	}
	return false
}

func newEventProcessor(conf *config.AgentConfig) *event.Processor {
	extractors := []event.Extractor{event.NewMetricBasedExtractor()}
	if len(conf.AnalyzedSpansByService) > 0 {
		extractors = append(extractors, event.NewFixedRateExtractor(conf.AnalyzedSpansByService))
	} else if len(conf.AnalyzedRateByServiceLegacy) > 0 {
		extractors = append(extractors, event.NewLegacyExtractor(conf.AnalyzedRateByServiceLegacy))
	}

	return event.NewProcessor(extractors, conf.MaxEPS)
}

// processedTrace creates a ProcessedTrace based on the provided chunk and root.
func processedTrace(p *api.Payload, chunk *pb.TraceChunk, root *pb.Span) *traceutil.ProcessedTrace {
	return &traceutil.ProcessedTrace{
		TraceChunk:             chunk,
		Root:                   root,
		AppVersion:             p.TracerPayload.AppVersion,
		TracerEnv:              p.TracerPayload.Env,
		TracerHostname:         p.TracerPayload.Hostname,
		ClientDroppedP0sWeight: float64(p.ClientDroppedP0s) / float64(len(p.Chunks())),
	}
}

// newChunksArray creates a new array which will point only to sampled chunks.

// The underlying array behind TracePayload.Chunks points to unsampled chunks
// preventing them from being collected by the GC.
func newChunksArray(chunks []*pb.TraceChunk) []*pb.TraceChunk {
	//nolint:revive // TODO(APM) Fix revive linter
	new := make([]*pb.TraceChunk, len(chunks))
	copy(new, chunks)
	return new
}

func mergeDuplicates(s *pb.ClientStatsBucket) {
	indexes := make(map[stats.Aggregation]int, len(s.Stats))
	for i, g := range s.Stats {
		a := stats.NewAggregationFromGroup(g)
		if j, ok := indexes[a]; ok {
			s.Stats[j].Hits += g.Hits
			s.Stats[j].Errors += g.Errors
			s.Stats[j].Duration += g.Duration
			s.Stats[i].Hits = 0
			s.Stats[i].Errors = 0
			s.Stats[i].Duration = 0
		} else {
			indexes[a] = i
		}
	}
}

func isManualUserDrop(priority sampler.SamplingPriority, pt *traceutil.ProcessedTrace) bool {
	if priority != sampler.PriorityUserDrop {
		return false
	}
	dm, hasDm := pt.Root.Meta[tagDecisionMaker]
	if !hasDm {
		return false
	}
	return dm == manualSampling
}

// setChunkAttributesFromRoot takes a trace chunk and from the root span
// * populates Origin field if it wasn't populated
// * populates Priority field if it wasn't populated
func setChunkAttributesFromRoot(chunk *pb.TraceChunk, root *pb.Span) {
	// check if priority is already populated
	if chunk.Priority == int32(sampler.PriorityNone) {
		// Older tracers set sampling priority in the root span.
		if p, ok := root.Metrics[tagSamplingPriority]; ok {
			chunk.Priority = int32(p)
		} else {
			for _, s := range chunk.Spans {
				if p, ok := s.Metrics[tagSamplingPriority]; ok {
					chunk.Priority = int32(p)
					break
				}
			}
		}
	}
	if chunk.Origin == "" && root.Meta != nil {
		// Older tracers set origin in the root span.
		chunk.Origin = root.Meta[tagOrigin]
	}
}

func newCreditCardsObfuscator(cfg config.CreditCardsConfig) *ccObfuscator {
	cco := &ccObfuscator{luhn: cfg.Luhn}
	if cfg.Enabled {
		// obfuscator disabled
		pb.SetMetaHook(cco.MetaHook)
	}
	return cco
}

func isValidStatusCode(sc string) bool {
	if code, err := strconv.ParseUint(sc, 10, 64); err == nil {
		return 100 <= code && code < 600
	}
	return false
}
