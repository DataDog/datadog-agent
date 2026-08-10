// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stats provides statistical utilities for the trace package.
package stats

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const keyStatsComputed = "_dd.stats_computed"

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
	primaryTagKeys []string,
) []stats.Input {
	return OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, containerTagKeys, peerTagKeys, primaryTagKeys, nil)
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
	primaryTagKeys []string,
	obfuscator *obfuscate.Obfuscator,
) []stats.Input {
	resourceSpans := traces.ResourceSpans()
	var clientComputedResources map[pcommon.Resource]struct{}
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		if resourceSpansHasClientComputedStats(resourceSpan) {
			if clientComputedResources == nil {
				clientComputedResources = make(map[pcommon.Resource]struct{})
			}
			clientComputedResources[resourceSpan.Resource()] = struct{}{}
		}
	}
	if resourceSpans.Len() > 0 && len(clientComputedResources) == resourceSpans.Len() {
		return nil
	}

	spanByID, resByID, scopeByID := transform.IndexOTelSpans(traces)
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	topLevelSpans := transform.GetTopLevelOTelSpans(spanByID, resByID, topLevelByKind)
	ignoreResNames := make(map[string]struct{})
	for _, resName := range conf.Ignore["resource"] {
		ignoreResNames[resName] = struct{}{}
	}
	chunks := make(map[chunkKey]*pb.TraceChunk)
	containerTagsByID := make(map[string][]string)
	for spanID, otelspan := range spanByID {
		otelres := resByID[spanID]
		if _, ok := clientComputedResources[otelres]; ok {
			continue
		}
		var resourceName string
		if transform.OperationAndResourceNameV2Enabled(conf) {
			resourceName = transform.GetOTelResourceV2(otelspan, otelres)
		} else {
			resourceName = transform.GetOTelResourceV1(otelspan, otelres)
		}
		if _, exists := ignoreResNames[resourceName]; exists {
			continue
		}

		env := transform.GetOTelEnv(otelspan, otelres)
		hostname := transform.GetOTelHostname(otelspan, otelres, conf.OTLPReceiver.AttributesTranslator, conf.Hostname)
		version := transform.GetOTelVersion(otelspan, otelres)
		var cid string
		if !conf.HasFeature("disable_otlp_container_tags_v2") {
			cid = transform.GetOTelContainerID(otelspan, otelres)
		} else {
			cid = transform.GetOTelContainerOrPodID(otelspan, otelres)
		}
		var ctags []string
		if cid != "" {
			ctags = transform.GetOTelContainerTags(otelres.Attributes(), containerTagKeys)
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
			traceIDUInt64: transform.OTelTraceIDToUint64(otelspan.TraceID()),
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
		ddSpan := transform.OtelSpanToDDSpanMinimal(otelspan, otelres, scopeByID[spanID], isTop, topLevelByKind, conf, peerTagKeys, primaryTagKeys)
		if obfuscator != nil {
			obfuscateSpanForConcentrator(obfuscator, ddSpan, conf)
		}
		chunk.Spans = append(chunk.Spans, ddSpan)
	}

	inputs := make([]stats.Input, 0, len(chunks))
	for ckey, chunk := range chunks {
		pt := traceutil.ProcessedTrace{
			TraceChunk:     chunk,
			Root:           traceutil.GetRoot(chunk.Spans),
			TracerEnv:      ckey.env,
			AppVersion:     ckey.version,
			TracerHostname: ckey.hostname,
		}
		inputs = append(inputs, stats.Input{
			Traces:        []traceutil.ProcessedTrace{pt},
			ContainerID:   ckey.cid,
			ContainerTags: containerTagsByID[ckey.cid],
		})
	}
	return inputs
}

func resourceSpansHasClientComputedStats(resourceSpans ptrace.ResourceSpans) bool {
	if hasClientComputedStats(resourceSpans.Resource().Attributes()) {
		return true
	}
	for i := 0; i < resourceSpans.ScopeSpans().Len(); i++ {
		spans := resourceSpans.ScopeSpans().At(i).Spans()
		for j := 0; j < spans.Len(); j++ {
			if hasClientComputedStats(spans.At(j).Attributes()) {
				return true
			}
		}
	}
	return false
}

func hasClientComputedStats(attrs pcommon.Map) bool {
	if _, ok := attrs.Get(keyStatsComputed); !ok {
		return false
	}
	value := transform.GetOTelAttrVal(attrs, true, keyStatsComputed)
	return value != "" && value != "false"
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
