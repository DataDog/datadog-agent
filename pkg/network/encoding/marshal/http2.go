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

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

type http2Encoder struct {
	http2AggregationsBuilder *model.HTTP2AggregationsBuilder
}

func newHTTP2Encoder() *http2Encoder {
	return &http2Encoder{
		http2AggregationsBuilder: model.NewHTTP2AggregationsBuilder(nil),
	}
}

func (e *http2Encoder) WriteHTTP2AggregationsAndTags(c network.ConnectionStats, builder *model.ConnectionBuilder) (uint64, map[string]struct{}) {
	if len(c.HTTP2Stats) == 0 {
		return 0, nil
	}

	var (
		staticTags  uint64
		dynamicTags map[string]struct{}
	)

	builder.SetHttp2Aggregations(func(b *bytes.Buffer) {
		staticTags, dynamicTags = e.encodeData(c.HTTP2Stats, b)
	})
	return staticTags, dynamicTags
}

func (e *http2Encoder) encodeData(connectionData []network.USMKeyValue[http.Key, *http.RequestStats], w io.Writer) (uint64, map[string]struct{}) {
	var staticTags uint64
	dynamicTags := make(map[string]struct{})
	e.http2AggregationsBuilder.Reset(w)

	for _, kvPair := range connectionData {
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

							blob, _ := proto.Marshal(latencies.ToProto())
							w.SetLatencies(func(b *bytes.Buffer) {
								b.Write(blob)
							})
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
}
