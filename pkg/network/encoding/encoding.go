// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"strings"
	"sync"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
			NpmEnabled: config.SystemProbe.GetBool("network_config.enabled"),
			UsmEnabled: config.SystemProbe.GetBool("service_monitoring_config.enabled"),
			DsmEnabled: config.SystemProbe.GetBool("data_streams_config.enabled"),
		}
	})

	agentConns := make([]*model.Connection, len(conns.Conns))
	routeIndex := make(map[string]RouteIdx)
	httpEncoder := newHTTPEncoder(conns)
	kafkaEncoder := newKafkaEncoder(conns)
	http2Encoder := newHTTP2Encoder(conns)
	ipc := make(ipCache, len(conns.Conns)/2)
	dnsFormatter := newDNSFormatter(conns, ipc)
	tagsSet := network.NewTagsSet()

	for i, conn := range conns.Conns {
		agentConns[i] = FormatConnection(conn, routeIndex, httpEncoder, http2Encoder, kafkaEncoder, dnsFormatter, ipc, tagsSet)
	}

	httpOrphan := httpEncoder.OrphanAggregations()

	if httpOrphan > 0 {
		log.Debugf(
			"detected orphan http aggregations. this may be caused by conntrack sampling or missed tcp close events. count=%d",
			httpOrphan,
		)

		telemetry.NewMetric(
			"usm.http.orphan_aggregations",
			telemetry.OptMonotonic,
			telemetry.OptExpvar,
			telemetry.OptStatsd,
		).Add(int64(httpOrphan))
	}

	if http2Encoder != nil && http2Encoder.orphanEntries > 0 {
		log.Debugf(
			"detected orphan http2 aggregations. this may be caused by conntrack sampling or missed tcp close events. count=%d",
			http2Encoder.orphanEntries,
		)

		telemetry.NewMetric(
			"usm.http2.orphan_aggregations",
			telemetry.OptMonotonic,
			telemetry.OptExpvar,
			telemetry.OptStatsd,
		).Add(int64(http2Encoder.orphanEntries))
	}

	if kafkaEncoder != nil && kafkaEncoder.orphanEntries > 0 {
		log.Debugf(
			"detected orphan kafka aggregations. this may be caused by conntrack sampling or missed tcp close events. count=%d",
			kafkaEncoder.orphanEntries,
		)

		telemetry.NewMetric(
			"usm.kafka.orphan_aggregations",
			telemetry.OptMonotonic,
			telemetry.OptExpvar,
			telemetry.OptStatsd,
		).Add(int64(kafkaEncoder.orphanEntries))
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
	payload.ConnTelemetryMap = FormatConnectionTelemetry(conns.ConnTelemetry)
	payload.CompilationTelemetryByAsset = FormatCompilationTelemetry(conns.CompilationTelemetryByAsset)
	payload.KernelHeaderFetchResult = model.KernelHeaderFetchResult(conns.KernelHeaderFetchResult)
	payload.CORETelemetryByAsset = FormatCORETelemetry(conns.CORETelemetryByAsset)
	payload.PrebuiltEBPFAssets = conns.PrebuiltAssets
	payload.Routes = routes
	payload.Tags = tagsSet.GetStrings()

	return payload
}
