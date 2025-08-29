// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"strings"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/tinylib/msgp/msgp"
)

// GetMediaTypeValue attempts to parse a Content-Type header value and returns the
// media type. If parsing fails, it returns the default "application/json".
func GetMediaTypeValue(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Debugf(`Error parsing media type: %v, assuming "application/json"`, err)
		return "application/json"
	}
	return mt
}

// DecodeTracerPayloadBytes decodes an incoming tracer payload from a byte slice,
// based on the API version and media type. It enriches the payload with language
// metadata and container ID in a transport-agnostic way.
func DecodeTracerPayloadBytes(
	v Version,
	mediaType string,
	body []byte,
	containerID string,
	lang string,
	langVersion string,
	tracerVersion string,
) (*pb.TracerPayload, error) {
	switch v {
	case v01:
		var spans []*pb.Span
		if err := json.Unmarshal(body, &spans); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     containerID,
			Chunks:          traceChunksFromSpans(spans),
			TracerVersion:   tracerVersion,
		}, nil
	case v05:
		var traces pb.Traces
		if err := traces.UnmarshalMsgDictionary(body); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     containerID,
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   tracerVersion,
		}, nil
	case V07:
		var tracerPayload pb.TracerPayload
		if _, err := tracerPayload.UnmarshalMsg(body); err != nil {
			return nil, err
		}
		return &tracerPayload, nil
	default:
		var traces pb.Traces
		if err := decodeTracesBytes(mediaType, body, &traces); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     containerID,
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   tracerVersion,
		}, nil
	}
}

// decodeTracesBytes decodes traces from bytes based on the media type.
func decodeTracesBytes(mediaType string, body []byte, dest *pb.Traces) error {
	switch mediaType {
	case "application/msgpack":
		_, err := dest.UnmarshalMsg(body)
		return err
	case "application/json", "text/json", "":
		return json.Unmarshal(body, &dest)
	default:
		// do our best: try json first, then msgpack
		if err := json.Unmarshal(body, &dest); err != nil {
			_, err2 := dest.UnmarshalMsg(body)
			return err2
		}
		return nil
	}
}

// BuildRateByServiceJSON computes the JSON body of the rate-by-service response.
// If ratesVersion equals the current state version, an empty JSON object is returned.
// It also returns the current version string for the caller to set in headers.
func BuildRateByServiceJSON(ratesVersion string, dynConf *sampler.DynamicConfig) (body []byte, currentVersion string, err error) {
	currentState := dynConf.RateByService.GetNewState(ratesVersion)
	currentVersion = currentState.Version
	if ratesVersion != "" && ratesVersion == currentVersion {
		return []byte("{}"), currentVersion, nil
	}
	response := struct {
		// All the sampling rates recommended, by service
		Rates map[string]float64 `json:"rate_by_service"`
	}{
		Rates: currentState.Rates,
	}
	body, err = json.Marshal(response)
	return body, currentVersion, err
}

// getProcessTagsFromMeta extracts process-level tags from a tracer payload,
// falling back to the provided process header value if not present in the payload.
// todo:raphael cleanup unused methods of extraction once implementation
// in all tracers is completed
// order of priority:
// 1. tags in the v07 payload
// 2. tags in the first span of the first chunk
// 3. tags in the header
func getProcessTagsFromMeta(processHeader string, p *pb.TracerPayload) string {
	if p == nil {
		return processHeader
	}
	if p.Tags != nil {
		if ptags, ok := p.Tags[tagProcessTags]; ok {
			return ptags
		}
	}
	if span, ok := getFirstSpan(p); ok {
		if ptags, ok := span.Meta[tagProcessTags]; ok {
			return ptags
		}
	}
	return processHeader
}

// getFirstSpan returns the first non-nil span in the first non-empty chunk.
func getFirstSpan(p *pb.TracerPayload) (*pb.Span, bool) {
	if len(p.Chunks) == 0 {
		return nil, false
	}
	for _, chunk := range p.Chunks {
		if chunk == nil || len(chunk.Spans) == 0 {
			continue
		}
		if chunk.Spans[0] == nil {
			continue
		}
		return chunk.Spans[0], true
	}
	return nil, false
}

// traceChunksFromSpans groups spans by TraceID into chunks.
func traceChunksFromSpans(spans []*pb.Span) []*pb.TraceChunk {
	traceChunks := []*pb.TraceChunk{}
	byID := make(map[uint64][]*pb.Span)
	for _, s := range spans {
		byID[s.TraceID] = append(byID[s.TraceID], s)
	}
	for _, t := range byID {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    t,
			Tags:     make(map[string]string),
		})
	}
	return traceChunks
}

// traceChunksFromTraces converts traces to chunks with default priority and empty tags.
func traceChunksFromTraces(traces pb.Traces) []*pb.TraceChunk {
	traceChunks := make([]*pb.TraceChunk, 0, len(traces))
	for _, trace := range traces {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    trace,
			Tags:     make(map[string]string),
		})
	}
	return traceChunks
}

// DecodeClientStatsPayloadBytes decodes a msgpack-encoded ClientStatsPayload from bytes.
func DecodeClientStatsPayloadBytes(body []byte) (*pb.ClientStatsPayload, error) {
	in := &pb.ClientStatsPayload{}
	if err := msgp.Decode(bytes.NewReader(body), in); err != nil {
		return nil, err
	}
	return in, nil
}

// FirstServiceFromClientStats returns the first service name found or empty string.
func FirstServiceFromClientStats(cs *pb.ClientStatsPayload) string {
	if cs == nil || len(cs.Stats) == 0 || len(cs.Stats[0].Stats) == 0 {
		return ""
	}
	return cs.Stats[0].Stats[0].Service
}

// UpdateTagStatsAccepted updates TagStats to reflect an accepted tracer payload.
func UpdateTagStatsAccepted(ts *info.TagStats, tp *pb.TracerPayload, bytes int64) {
	if ts == nil || tp == nil {
		return
	}
	ts.TracesReceived.Add(int64(len(tp.Chunks)))
	ts.TracesBytes.Add(bytes)
	ts.PayloadAccepted.Inc()
}

// BuildPayloadFromTracerPayload builds a Payload object from an existing tracer payload and tag stats
// without mutating tracer payload tags. Container and process tags are attached to the Payload struct
// for downstream use.
func BuildPayloadFromTracerPayload(tp *pb.TracerPayload, ts *info.TagStats, processTags string, containerTags []string) *Payload {
	return &Payload{
		Source:        ts,
		TracerPayload: tp,
		ProcessTags:   processTags,
		ContainerTags: containerTags,
	}
}

// DecodeAndProcessClientStats decodes a msgpack-encoded ClientStatsPayload from bytes,
// records transport-agnostic statsd metrics using TagStats, and invokes the provided
// StatsProcessor. It returns any error encountered (decode or processing).
func DecodeAndProcessClientStats(
	ctx context.Context,
	body []byte,
	lang string,
	langVersion string,
	interpreter string,
	langVendor string,
	tracerVersion string,
	obfuscationVersion string,
	containerID string,
	getTagStats func(info.Tags) *info.TagStats,
	statsd statsd.ClientInterface,
	processor StatsProcessor,
) error {
	in := &pb.ClientStatsPayload{}
	if err := msgp.Decode(bytes.NewReader(body), in); err != nil {
		// rejection on decode
		ts := getTagStats(info.Tags{
			Lang:            lang,
			LangVersion:     langVersion,
			Interpreter:     interpreter,
			LangVendor:      langVendor,
			TracerVersion:   tracerVersion,
			EndpointVersion: string(V06),
			Service:         "",
		})
		_ = statsd.Count("datadog.trace_agent.receiver.stats_payload_rejected", 1, append(ts.AsTags(), "reason:decode"), 1)
		return err
	}

	ts := getTagStats(info.Tags{
		Lang:            lang,
		LangVersion:     langVersion,
		Interpreter:     interpreter,
		LangVendor:      langVendor,
		TracerVersion:   tracerVersion,
		EndpointVersion: string(V06),
		Service:         FirstServiceFromClientStats(in),
	})
	_ = statsd.Count("datadog.trace_agent.receiver.stats_payload", 1, ts.AsTags(), 1)
	_ = statsd.Count("datadog.trace_agent.receiver.stats_bytes", int64(len(body)), ts.AsTags(), 1)
	_ = statsd.Count("datadog.trace_agent.receiver.stats_buckets", int64(len(in.Stats)), ts.AsTags(), 1)

	if err := processor.ProcessStats(ctx, in, lang, tracerVersion, containerID, obfuscationVersion); err != nil {
		_ = statsd.Count("datadog.trace_agent.receiver.stats_payload_rejected", 1, append(ts.AsTags(), "reason:timeout"), 1)
		return err
	}
	return nil
}

// BuildPayloadAndRecordStats decodes a tracer payload, applies process/container tags,
// updates TagStats counters and records timing metrics. It returns the built Payload and TagStats.
// It does not set client-computed flags or dropped P0 counts which are transport-specific.
func BuildPayloadAndRecordStats(
	v Version,
	mediaType string,
	body []byte,
	containerID string,
	lang string,
	langVersion string,
	interpreter string,
	langVendor string,
	tracerVersion string,
	processHeader string,
	containerTagsFn func(string) ([]string, error),
	getTagStats func(info.Tags) *info.TagStats,
	traceCount int64,
	statsd statsd.ClientInterface,
	start time.Time,
) (*Payload, *info.TagStats, error) {
	// We'll construct TagStats with empty service for error cases; recompute after successful decode.
	ts := getTagStats(info.Tags{
		Lang:            lang,
		LangVersion:     langVersion,
		Interpreter:     interpreter,
		LangVendor:      langVendor,
		TracerVersion:   tracerVersion,
		EndpointVersion: string(v),
		Service:         "",
	})

	// Decode payload
	tp, err := DecodeTracerPayloadBytes(v, mediaType, body, containerID, lang, langVersion, tracerVersion)
	if err != nil {
		// Classify drop reason
		switch err {
		case io.EOF, io.ErrUnexpectedEOF:
			ts.TracesDropped.EOF.Add(traceCount)
		case msgp.ErrShortBytes:
			ts.TracesDropped.MSGPShortBytes.Add(traceCount)
		default:
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				ts.TracesDropped.Timeout.Add(traceCount)
			} else {
				ts.TracesDropped.DecodingError.Add(traceCount)
			}
		}
		// record failure timing
		tags := append(ts.AsTags(), "success:false")
		_ = statsd.Histogram("datadog.trace_agent.receiver.serve_traces_ms", float64(time.Since(start))/float64(time.Millisecond), tags, 1)
		return nil, ts, err
	}

	// Recompute TagStats with service name after successful decode
	service := ""
	if fs, ok := getFirstSpan(tp); ok {
		service = fs.Service
	}
	ts = getTagStats(info.Tags{
		Lang:            lang,
		LangVersion:     langVersion,
		Interpreter:     interpreter,
		LangVendor:      langVendor,
		TracerVersion:   tracerVersion,
		EndpointVersion: string(v),
		Service:         service,
	})

	// Mark success timing
	tags := append(ts.AsTags(), "success:true")
	_ = statsd.Histogram("datadog.trace_agent.receiver.serve_traces_ms", float64(time.Since(start))/float64(time.Millisecond), tags, 1)

	// Update TagStats counters
	ts.TracesReceived.Add(int64(len(tp.Chunks)))
	ts.TracesBytes.Add(int64(len(body)))
	ts.PayloadAccepted.Inc()

	// Apply container tags to tracer payload
	ctags := getContainerTagsList(containerTagsFn, containerID)
	if len(ctags) > 0 {
		if tp.Tags == nil {
			tp.Tags = make(map[string]string)
		}
		tp.Tags[tagContainersTags] = strings.Join(ctags, ",")
	}

	// Apply process tags
	ptags := getProcessTagsFromMeta(processHeader, tp)
	if ptags != "" {
		if tp.Tags == nil {
			tp.Tags = make(map[string]string)
		}
		tp.Tags[tagProcessTags] = ptags
	}

	payload := &Payload{
		Source:                 ts,
		TracerPayload:          tp,
		ClientComputedTopLevel: false,
		ClientComputedStats:    false,
		ClientDroppedP0s:       0,
		ProcessTags:            ptags,
		ContainerTags:          ctags,
	}
	return payload, ts, nil
}
