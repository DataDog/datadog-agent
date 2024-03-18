// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

type http2Encoder struct {
	http2AggregationsBuilder     *model.HTTP2AggregationsBuilder
	byConnection                 *USMConnectionIndex[http.Key, *http.RequestStats]
	enableCustomDDSketchEncoding bool
	sketchBuffer                 *[]byte
}

func newHTTP2Encoder(http2Payloads map[http.Key]*http.RequestStats) *http2Encoder {
	if len(http2Payloads) == 0 {
		return nil
	}

	return &http2Encoder{
		byConnection: GroupByConnection("http2", http2Payloads, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
		http2AggregationsBuilder:     model.NewHTTP2AggregationsBuilder(nil),
		enableCustomDDSketchEncoding: coreconfig.SystemProbe.GetBool(customDDSketchEncodingCfg),
	}
}

func (e *http2Encoder) WriteHTTP2AggregationsAndTags(c network.ConnectionStats, builder *model.ConnectionBuilder) (uint64, map[string]struct{}) {
	if e == nil {
		return 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0, nil
	}

	var (
		staticTags  uint64
		dynamicTags map[string]struct{}
	)

	builder.SetHttp2Aggregations(func(b *bytes.Buffer) {
		staticTags, dynamicTags = e.encodeData(connectionData, b)
	})
	return staticTags, dynamicTags
}

func (e *http2Encoder) encodeData(connectionData *USMConnectionData[http.Key, *http.RequestStats], w io.Writer) (uint64, map[string]struct{}) {
	var staticTags uint64
	dynamicTags := make(map[string]struct{})
	e.http2AggregationsBuilder.Reset(w)

	for _, kvPair := range connectionData.Data {
		e.http2AggregationsBuilder.AddEndpointAggregations(func(http2StatsBuilder *model.HTTPStatsBuilder) {
			key := kvPair.Key
			stats := kvPair.Value

			http2StatsBuilder.SetPath(key.Path.Content.Get())
			http2StatsBuilder.SetFullPath(key.Path.FullPath)
			http2StatsBuilder.SetMethod(uint64(model.HTTPMethod(key.Method)))

			for code, stats := range stats.Data {
				http2StatsBuilder.AddStatsByStatusCode(func(w *model.HTTPStats_StatsByStatusCodeEntryBuilder) {
					w.SetKey(int32(code))
					w.SetValue(func(w *model.HTTPStats_DataBuilder) {
						w.SetCount(uint32(stats.Count))
						if latencies := stats.Latencies; latencies != nil {
							if e.enableCustomDDSketchEncoding {
								if e.sketchBuffer == nil {
									tmp := make([]byte, 0, defaultSketchBufferSize)
									e.sketchBuffer = &tmp
								}
								w.SetEncodedLatencies(func(b *bytes.Buffer) {
									latencies.Encode(e.sketchBuffer, false)
									b.Write(*e.sketchBuffer)
									*e.sketchBuffer = (*e.sketchBuffer)[:0]
								})
							} else {
								blob, _ := proto.Marshal(latencies.ToProto())
								w.SetLatencies(func(b *bytes.Buffer) {
									b.Write(blob)
								})
							}
						} else {
							w.SetFirstLatencySample(stats.FirstLatencySample)
						}
					})
				})

				staticTags |= stats.StaticTags
				for _, dynamicTag := range stats.DynamicTags {
					dynamicTags[dynamicTag] = struct{}{}
				}
			}
		})
	}

	return staticTags, dynamicTags
}

func (e *http2Encoder) Close() {
	if e == nil {
		return
	}

	e.byConnection.Close()
}
