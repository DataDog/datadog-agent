// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/sketches-go/ddsketch"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

type httpEncoder struct {
	httpAggregationsBuilder *model.HTTPAggregationsBuilder
	byConnection            *USMConnectionIndex[http.Key, *http.RequestStats]
	sketchBuilder           *ddsketch.DDSketchCollectionBuilder
	discoveryMode           bool
}

func newHTTPEncoder(httpPayloads map[http.Key]*http.RequestStats) *httpEncoder {
	if len(httpPayloads) == 0 {
		return nil
	}

	return &httpEncoder{
		httpAggregationsBuilder: model.NewHTTPAggregationsBuilder(nil),
		sketchBuilder:           ddsketch.NewDDSketchCollectionBuilder(nil),
		discoveryMode:           pkgconfigsetup.SystemProbe().GetBool("discovery.service_map.enabled"),
		byConnection: GroupByConnection("http", httpPayloads, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
	}
}

func (e *httpEncoder) EncodeConnection(c network.ConnectionStats, builder *model.ConnectionBuilder) (staticTags uint64, dynamicTags map[string]struct{}) {
	builder.SetHttpAggregations(func(b *bytes.Buffer) {
		staticTags, dynamicTags = e.encodeData(c, b)
	})
	return
}

func (e *httpEncoder) encodeData(c network.ConnectionStats, w io.Writer) (uint64, map[string]struct{}) {
	if e == nil {
		return 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0, nil
	}

	var staticTags uint64
	dynamicTags := make(map[string]struct{})
	e.httpAggregationsBuilder.Reset(w)

	for _, kvPair := range connectionData.Data {
		e.httpAggregationsBuilder.AddEndpointAggregations(func(httpStatsBuilder *model.HTTPStatsBuilder) {
			encodeUSMEndpoint(httpStatsBuilder, kvPair.Key, kvPair.Value, e.discoveryMode, e.sketchBuilder, &staticTags, dynamicTags)
		})
	}
	return staticTags, dynamicTags
}

// encodeUSMEndpoint encodes one endpoint aggregation into the shared builder used
// by the HTTP and HTTP/2 encoders. Discovery mode drops path/method and uses
// LatencySum instead of a DDSketch.
func encodeUSMEndpoint(builder *model.HTTPStatsBuilder, key http.Key, stats *http.RequestStats, discoveryMode bool, sketchBuilder *ddsketch.DDSketchCollectionBuilder, staticTags *uint64, dynamicTags map[string]struct{}) {
	if !discoveryMode {
		builder.SetPath(key.Path.Content.Get())
		builder.SetFullPath(key.Path.FullPath)
		builder.SetMethod(uint64(model.HTTPMethod(key.Method)))
	}

	for code, stat := range stats.Data {
		builder.AddStatsByStatusCode(func(w *model.HTTPStats_StatsByStatusCodeEntryBuilder) {
			w.SetKey(int32(code))
			w.SetValue(func(w *model.HTTPStats_DataBuilder) {
				w.SetCount(uint32(stat.Count))
				if discoveryMode {
					w.SetLatencySum(stat.LatencySum)
				} else if latencies := stat.Latencies; latencies != nil {
					w.SetLatencies(func(b *bytes.Buffer) {
						sketchBuilder.Reset(b)
						sketchBuilder.AddSketch(latencies)
					})
				} else {
					w.SetFirstLatencySample(stat.FirstLatencySample)
				}
			})
		})

		*staticTags |= stat.StaticTags
		for dynamicTag := range stat.DynamicTags {
			dynamicTags[dynamicTag] = struct{}{}
		}
	}
}

func (e *httpEncoder) Close() {
	if e == nil {
		return
	}

	e.byConnection.Close()
}
