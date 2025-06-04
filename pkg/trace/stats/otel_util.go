// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"

	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// chunkKey is used to group TraceChunks
type chunkKey struct {
	traceIDUInt64 uint64
	env           string
	version       string
	hostname      string
	cid           string
}

// OTLPTracesToConcentratorInputs converts eligible OTLP spans to Concentrator.Input.
// The converted Inputs only have the minimal number of fields for APM stats calculation and are only meant
// to be used in Concentrator.Add(). Do not use them for other purposes.
func OTLPTracesToConcentratorInputs(
	traces ptrace.Traces,
	conf *config.AgentConfig,
	containerTagKeys []string,
	peerTagKeys []string,
) []Input {
	return OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, containerTagKeys, peerTagKeys, nil)
}

// OTLPTracesToConcentratorInputsWithObfuscation converts eligible OTLP spans to Concentrator Input.
// The converted Inputs only have the minimal number of fields for APM stats calculation and are only meant
// to be used in Concentrator.Add(). Do not use them for other purposes.
// This function enables obfuscation of spans prior to stats calculation and datadogconnector will migrate
// to this function once this function is published as part of latest pkg/trace module.
func OTLPTracesToConcentratorInputsWithObfuscation(
	traces ptrace.Traces,
	conf *config.AgentConfig,
	containerTagKeys []string,
	peerTagKeys []string,
	obfuscator *obfuscate.Obfuscator,
) []Input {
	spanByID, resByID, scopeByID := traceutil.IndexOTelSpans(traces)
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	topLevelSpans := traceutil.GetTopLevelOTelSpans(spanByID, resByID, topLevelByKind)
	ignoreResNames := make(map[string]struct{})
	for _, resName := range conf.Ignore["resource"] {
		ignoreResNames[resName] = struct{}{}
	}
	chunks := make(map[chunkKey]*pb.TraceChunk)
	containerTagsByID := make(map[string][]string)
	for spanID, otelspan := range spanByID {
		otelres := resByID[spanID]
		var resourceName string
		if transform.OperationAndResourceNameV2Enabled(conf) {
			resourceName = traceutil.GetOTelResourceV2(otelspan, otelres)
		} else {
			resourceName = traceutil.GetOTelResourceV1(otelspan, otelres)
		}
		if _, exists := ignoreResNames[resourceName]; exists {
			continue
		}

		env := transform.GetOTelEnv(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		hostname := transform.GetOTelHostname(otelspan, otelres, conf.OTLPReceiver.AttributesTranslator, conf.Hostname, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		version := transform.GetOTelVersion(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		cid := transform.GetOTelContainerID(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		var ctags []string
		if cid != "" {
			ctags = traceutil.GetOTelContainerTags(otelres.Attributes(), containerTagKeys)
			if conf.ContainerTags != nil {
				tags, err := conf.ContainerTags(cid)
				if err != nil {
					log.Debugf("Failed to get container tags for container %q: %v", cid, err)
				} else {
					log.Tracef("Getting container tags for ID %q: %v", cid, tags)
					ctags = append(ctags, tags...)
				}
			}
			if ctags != nil {
				// Make sure container tags are sorted per APM stats intake requirement
				if !slices.IsSorted(ctags) {
					slices.Sort(ctags)
				}
				containerTagsByID[cid] = ctags
			}
		}
		ckey := chunkKey{
			traceIDUInt64: traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
			env:           env,
			version:       version,
			hostname:      hostname,
			cid:           cid,
		}
		chunk, ok := chunks[ckey]
		if !ok {
			chunk = &pb.TraceChunk{}
			chunks[ckey] = chunk
		}
		_, isTop := topLevelSpans[spanID]
		ddSpan := transform.OtelSpanToDDSpanMinimal(otelspan, otelres, scopeByID[spanID], isTop, topLevelByKind, conf, peerTagKeys)
		if obfuscator != nil {
			obfuscateSpanForConcentrator(obfuscator, ddSpan, conf)
		}
		chunk.Spans = append(chunk.Spans, ddSpan)
	}

	inputs := make([]Input, 0, len(chunks))
	for ckey, chunk := range chunks {
		pt := traceutil.ProcessedTrace{
			TraceChunk:     chunk,
			Root:           traceutil.GetRoot(chunk.Spans),
			TracerEnv:      ckey.env,
			AppVersion:     ckey.version,
			TracerHostname: ckey.hostname,
		}
		inputs = append(inputs, Input{
			Traces:        []traceutil.ProcessedTrace{pt},
			ContainerID:   ckey.cid,
			ContainerTags: containerTagsByID[ckey.cid],
		})
	}
	return inputs
}

func obfuscateSpanForConcentrator(o *obfuscate.Obfuscator, span *pb.Span, conf *config.AgentConfig) {
	if span.Meta == nil {
		return
	}
	switch span.Type {
	case "sql", "cassandra":
		_, err := transform.ObfuscateSQLSpan(o, span)
		if err != nil {
			log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
		}
	case "redis":
		span.Resource = o.QuantizeRedisString(span.Resource)
		if conf.Obfuscation.Redis.Enabled {
			transform.ObfuscateRedisSpan(o, span, conf.Obfuscation.Redis.RemoveAllArgs)
		}
	}
}

// newTestObfuscator creates a new obfuscator for testing
func newTestObfuscator(conf *config.AgentConfig) *obfuscate.Obfuscator {
	oconf := conf.Obfuscation.Export(conf)
	oconf.Redis.Enabled = true
	o := obfuscate.NewObfuscator(oconf)
	return o
}
