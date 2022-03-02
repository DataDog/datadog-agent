// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"strings"
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gogo/protobuf/jsonpb"
)

var (
	pSerializer = protoSerializer{}
	jSerializer = jsonSerializer{
		marshaller: jsonpb.Marshaler{
			EmitDefaults: true,
		},
	}

	cfgOnce  = sync.Once{}
	agentCfg *model.AgentConfiguration

	httpAggregationsPool = sync.Pool{
		New: func() interface{} {
			return &model.HTTPAggregations{
				EndpointAggregations: make([]*model.HTTPStats, 0, 5),
			}
		},
	}

	httpStatsPool = sync.Pool{
		New: func() interface{} {
			ms := &model.HTTPStats{
				StatsByResponseStatus: make([]*model.HTTPStats_Data, http.NumStatusClasses),
			}

			for i := range ms.StatsByResponseStatus {
				ms.StatsByResponseStatus[i] = &model.HTTPStats_Data{}
			}

			return ms
		},
	}
)

// Marshaler is an interface implemented by all Connections serializers
type Marshaler interface {
	Marshal(conns *network.Connections) ([]byte, error)
	ContentType() string
}

// Unmarshaler is an interface implemented by all Connections deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*model.Connections, error)
}

// GetMarshaler returns the appropriate Marshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

func modelConnections(conns *network.Connections) *model.Connections {
	cfgOnce.Do(func() {
		agentCfg = &model.AgentConfiguration{
			NpmEnabled: config.Datadog.GetBool("network_config.enabled"),
			TsmEnabled: config.Datadog.GetBool("service_monitoring_config.enabled"),
		}
	})

	agentConns := make([]*model.Connection, len(conns.Conns))
	routeIndex := make(map[string]RouteIdx)
	httpIndex := FormatHTTPStats(conns.HTTP)
	httpMatches := make(map[http.Key]struct{}, len(httpIndex))
	ipc := make(ipCache, len(conns.Conns)/2)
	dnsFormatter := newDNSFormatter(conns, ipc)

	for i, conn := range conns.Conns {
		httpKey := httpKeyFromConn(conn)
		httpAggregations := httpIndex[httpKey]
		if httpAggregations != nil {
			httpMatches[httpKey] = struct{}{}
		}

		agentConns[i] = FormatConnection(conn, routeIndex, httpAggregations, dnsFormatter, ipc)
	}

	// return HTTPAggregation objects to pool
	for _, aggr := range httpIndex {
		resetHTTPAggregations(aggr)
		httpAggregationsPool.Put(aggr)
	}

	if orphans := len(httpIndex) - len(httpMatches); orphans > 0 {
		log.Debugf(
			"detected orphan http aggreggations. this can be either caused by conntrack sampling or missed tcp close events. count=%d",
			orphans,
		)
	}

	routes := make([]*model.Route, len(routeIndex))
	for _, v := range routeIndex {
		routes[v.Idx] = &v.Route
	}

	payload := new(model.Connections)
	payload.AgentConfiguration = agentCfg
	payload.Conns = agentConns
	payload.Domains = dnsFormatter.Domains()
	payload.Dns = dnsFormatter.DNS()
	payload.ConnTelemetry = FormatConnTelemetry(conns.ConnTelemetry)
	payload.CompilationTelemetryByAsset = FormatCompilationTelemetry(conns.CompilationTelemetryByAsset)
	payload.Routes = routes

	return payload
}

func resetHTTPAggregations(aggr *model.HTTPAggregations) {
	for _, e := range aggr.EndpointAggregations {
		resetHTTPStats(e)
	}
	aggr.EndpointAggregations = aggr.EndpointAggregations[:0]
}

func resetHTTPStats(stats *model.HTTPStats) {
	for _, es := range stats.StatsByResponseStatus {
		es.Count = 0
		es.Latencies = nil
		es.FirstLatencySample = 0
	}

	httpStatsPool.Put(stats)
}
