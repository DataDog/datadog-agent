// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"io"
	"strings"
	"sync"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
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
	Marshal(conns *network.Connections, writer io.Writer) error
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

func modelConnections(builder *model.ConnectionsBuilder, conns *network.Connections) {
	cfgOnce.Do(func() {
		agentCfg = &model.AgentConfiguration{
			NpmEnabled: config.SystemProbe.GetBool("network_config.enabled"),
			UsmEnabled: config.SystemProbe.GetBool("service_monitoring_config.enabled"),
			DsmEnabled: config.SystemProbe.GetBool("data_streams_config.enabled"),
		}
	})

	routeIndex := make(map[string]RouteIdx)
	httpEncoder := newHTTPEncoder(conns)
	defer httpEncoder.Close()
	kafkaEncoder := newKafkaEncoder(conns)
	defer kafkaEncoder.Close()
	http2Encoder := newHTTP2Encoder(conns)
	defer http2Encoder.Close()

	ipc := make(ipCache, len(conns.Conns)/2)
	dnsFormatter := newDNSFormatter(conns, ipc)
	tagsSet := network.NewTagsSet()

	for _, conn := range conns.Conns {
		builder.AddConns(func(builder *model.ConnectionBuilder) {
			FormatConnection(builder, conn, routeIndex, httpEncoder, http2Encoder, kafkaEncoder, dnsFormatter, ipc, tagsSet)
		})
	}

	routes := make([]*model.Route, len(routeIndex))
	for _, v := range routeIndex {
		routes[v.Idx] = &v.Route
	}

	builder.SetAgentConfiguration(func(w *model.AgentConfigurationBuilder) {
		w.SetDsmEnabled(agentCfg.DsmEnabled)
		w.SetNpmEnabled(agentCfg.NpmEnabled)
		w.SetUsmEnabled(agentCfg.UsmEnabled)
	})
	for _, d := range dnsFormatter.Domains() {
		builder.AddDomains(d)
	}

	for _, route := range routes {
		builder.AddRoutes(func(w *model.RouteBuilder) {
			w.SetSubnet(func(w *model.SubnetBuilder) {
				w.SetAlias(route.Subnet.Alias)
			})
		})
	}

	dnsFormatter.FormatDNS(builder)

	for _, tag := range tagsSet.GetStrings() {
		builder.AddTags(tag)
	}

	FormatConnectionTelemetry(builder, conns.ConnTelemetry)
	FormatCompilationTelemetry(builder, conns.CompilationTelemetryByAsset)
	FormatCORETelemetry(builder, conns.CORETelemetryByAsset)
	builder.SetKernelHeaderFetchResult(uint64(conns.KernelHeaderFetchResult))
	for _, asset := range conns.PrebuiltAssets {
		builder.AddPrebuiltEBPFAssets(asset)
	}

}
