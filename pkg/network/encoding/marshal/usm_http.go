// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package marshal

import (
	"bytes"
	"io"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpEncoder struct {
	httpAggregationsBuilder *model.HTTPAggregationsBuilder
	byConnection            *USMConnectionIndex[http.Key, *http.RequestStats]
	sketchBuilder           *ddsketch.DDSketchCollectionBuilder
}

func newHTTPEncoder(httpPayloads map[http.Key]*http.RequestStats) *httpEncoder {
	if len(httpPayloads) == 0 {
		return nil
	}

	return &httpEncoder{
		httpAggregationsBuilder: model.NewHTTPAggregationsBuilder(nil),
		sketchBuilder:           ddsketch.NewDDSketchCollectionBuilder(nil),
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

// isK8sAPIPath checks if a path looks like a Kubernetes API path
func isK8sAPIPath(path string) bool {
	return strings.Contains(path, "persistentvolume") ||
		strings.Contains(path, "configmaps") ||
		strings.Contains(path, "namespaces")
}

func (e *httpEncoder) encodeData(c network.ConnectionStats, w io.Writer) (uint64, map[string]struct{}) {
	if e == nil {
		return 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0, nil
	}

	// TRACE: Log when NPM connection claims HTTP stats with k8s API paths
	if log.ShouldLog(log.TraceLvl) {
		for _, kvPair := range connectionData.Data {
			path := kvPair.Key.Path.Content.Get()
			if isK8sAPIPath(path) {
				log.Tracef("[USM-ENCODE-K8S-API] MATCH path=%s method=%v pid=%d conn=[%s:%d ⇄ %s:%d]",
					path, kvPair.Key.Method, c.Pid, c.Source, c.SPort, c.Dest, c.DPort)
			}
		}
	}

	var staticTags uint64
	dynamicTags := make(map[string]struct{})
	e.httpAggregationsBuilder.Reset(w)

	for _, kvPair := range connectionData.Data {
		e.httpAggregationsBuilder.AddEndpointAggregations(func(httpStatsBuilder *model.HTTPStatsBuilder) {
			key := kvPair.Key
			stats := kvPair.Value

			httpStatsBuilder.SetPath(key.Path.Content.Get())
			httpStatsBuilder.SetFullPath(key.Path.FullPath)
			httpStatsBuilder.SetMethod(uint64(model.HTTPMethod(key.Method)))

			for code, stats := range stats.Data {
				httpStatsBuilder.AddStatsByStatusCode(func(w *model.HTTPStats_StatsByStatusCodeEntryBuilder) {
					w.SetKey(int32(code))
					w.SetValue(func(w *model.HTTPStats_DataBuilder) {
						w.SetCount(uint32(stats.Count))
						if latencies := stats.Latencies; latencies != nil {
							w.SetLatencies(func(b *bytes.Buffer) {
								e.sketchBuilder.Reset(b)
								e.sketchBuilder.AddSketch(latencies)
							})
						} else {
							w.SetFirstLatencySample(stats.FirstLatencySample)
						}
					})
				})

				staticTags |= stats.StaticTags
				for dynamicTag := range stats.DynamicTags {
					dynamicTags[dynamicTag] = struct{}{}
				}
			}
		})

	}
	return staticTags, dynamicTags
}

func (e *httpEncoder) Close() {
	if e == nil {
		return
	}

	// TRACE: Log orphan k8s API paths (HTTP stats not claimed by any NPM connection)
	if log.ShouldLog(log.TraceLvl) {
		for key, value := range e.byConnection.GetData() {
			if !value.IsClaimed() {
				log.Tracef("[USM-ORPHAN-HTTP] key=%s dataLen=%d", key.String(), len(value.Data))
				for _, kvPair := range value.Data {
					path := kvPair.Key.Path.Content.Get()
					log.Tracef("[USM-ORPHAN-HTTP] path=%s method=%v isK8s=%v", path, kvPair.Key.Method, isK8sAPIPath(path))
					if isK8sAPIPath(path) {
						log.Tracef("[USM-ORPHAN-K8S-API] path=%s method=%v key=%s",
							path, kvPair.Key.Method, key.String())
					}
				}
			}
		}
	}

	e.byConnection.Close()
}
