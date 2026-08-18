// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

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

type http2Encoder struct {
	http2AggregationsBuilder *model.HTTP2AggregationsBuilder
	byConnection             *USMConnectionIndex[http.Key, *http.RequestStats]
	sketchBuilder            *ddsketch.DDSketchCollectionBuilder
	discoveryMode            bool
}

func newHTTP2Encoder(http2Payloads map[http.Key]*http.RequestStats) *http2Encoder {
	if len(http2Payloads) == 0 {
		return nil
	}

	return &http2Encoder{
		byConnection: GroupByConnection("http2", http2Payloads, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
		http2AggregationsBuilder: model.NewHTTP2AggregationsBuilder(nil),
		sketchBuilder:            ddsketch.NewDDSketchCollectionBuilder(nil),
		discoveryMode:            pkgconfigsetup.SystemProbe().GetBool("discovery.service_map.enabled"),
	}
}

func (e *http2Encoder) EncodeConnection(c network.ConnectionStats, builder *model.ConnectionBuilder) (staticTags uint64, dynamicTags map[string]struct{}) {
	builder.SetHttp2Aggregations(func(b *bytes.Buffer) {
		staticTags, dynamicTags = e.encodeData(c, b)
	})
	return
}

func (e *http2Encoder) encodeData(c network.ConnectionStats, w io.Writer) (uint64, map[string]struct{}) {
	if e == nil {
		return 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0, nil
	}

	var staticTags uint64
	dynamicTags := make(map[string]struct{})
	e.http2AggregationsBuilder.Reset(w)

	for _, kvPair := range connectionData.Data {
		e.http2AggregationsBuilder.AddEndpointAggregations(func(http2StatsBuilder *model.HTTPStatsBuilder) {
			encodeUSMEndpoint(http2StatsBuilder, kvPair.Key, kvPair.Value, e.discoveryMode, e.sketchBuilder, &staticTags, dynamicTags)
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
